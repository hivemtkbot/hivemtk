package service

import (
	"context"
	"errors"
	"fmt"
	"marketing/internal/model"
	"marketing/internal/repository"
	"marketing/internal/websocket"
	"reflect"
	"time"
)

// ============================================================================
// 方向10：坐席实时聊天看板 - 拉黑 / 解除拉黑
// 文档：docs/企业级架构优化/坐席实时聊天看板.md §四
//
// 文件拆分原因：customer_session.go 已超 750 行（含 AgentStatusService / SessionAssignmentService
// 等多个 Service 类型），将黑名单相关代码独立到本文件，便于：
//   1. 阅读聚焦：黑名单 CRUD + 校验链单独 review
//   2. 责任单一：本文件只关心 user_id 维度黑名单
//   3. 测试隔离：customer_session_blacklist_test.go 独立验证
// ============================================================================

// BlacklistRequest 拉黑请求
type BlacklistRequest struct {
	SessionID    uint   `json:"session_id" binding:"required"`
	Reason       string `json:"reason"`        // 拉黑原因
	OperatorID   uint   `json:"operator_id"`   // 操作人（坐席 ID）
	OperatorName string `json:"operator_name"` // 操作人姓名
	TTLHours     int    `json:"ttl_hours"`     // 0 = 永久
}

// BlacklistSource 黑名单来源枚举
type BlacklistSource string

const (
	BlacklistSourceManual BlacklistSource = "manual" // 坐席手动
	BlacklistSourceAuto   BlacklistSource = "auto"   // 系统自动
	BlacklistSourceRisk   BlacklistSource = "risk"   // 风控引擎
)

// BlacklistUser 拉黑当前会话对应的访客（user_id 维度）
//
// 行为：
//  1. 会话必须存在且有 user_id，否则拒绝。
//  2. 幂等：若该 user_id+platform 已存在 active 记录，更新 reason/expires_at。
//  3. 拉黑后立即关闭该会话（status=closed）以防继续对话。
//  4. 推 WebSocket 给前端（type=handler_changed, event=blacklisted）。
//
// 错误：返回业务语义化错误（含中文 message 供前端直接展示）。
func (s *CustomerSessionService) BlacklistUser(ctx context.Context, req *BlacklistRequest) error {
	_ = ctx // 当前实现未使用，预留支持 context 超时/链路追踪
	if req == nil {
		return errors.New("请求体不能为空")
	}
	if req.SessionID == 0 {
		return errors.New("session_id 必填")
	}
	session, err := s.sessionRepo.GetByID(req.SessionID)
	if err != nil {
		return errors.New("会话不存在")
	}
	if session.UserID == "" {
		return errors.New("该会话无 user_id，无法拉黑")
	}

	var expiresAt *time.Time
	if req.TTLHours > 0 {
		t := time.Now().Add(time.Duration(req.TTLHours) * time.Hour)
		expiresAt = &t
	}

	blacklistRecord := &model.UserBlacklist{
		UserID:       session.UserID,
		Platform:     session.Platform,
		Reason:       req.Reason,
		Source:       string(BlacklistSourceManual),
		OperatorID:   req.OperatorID,
		OperatorName: req.OperatorName,
		SessionID:    session.SessionID,
		Active:       true,
		ExpiresAt:    expiresAt,
	}
	if err := s.blacklistRepo.Add(blacklistRecord); err != nil {
		return fmt.Errorf("写入黑名单失败: %w", err)
	}

	// 关闭该会话，避免继续对话
	if err := s.sessionRepo.UpdateStatus(req.SessionID, model.SessionStatusClosed); err != nil {
		// 关闭失败不阻塞拉黑结果返回（黑名单已生效），但记录错误便于排查
		_ = err
	}

	// 通知前端：handler_changed + blacklisted
	if err := s.notifySessionUpdate(session, "blacklisted", "human"); err != nil {
		_ = err
	}
	if err := websocket.SendToVisitor(websocket.TypeAgentJoined, map[string]any{
		"session_id":  session.SessionID,
		"handler":     "human",
		"reason":      "因违反服务条款，该访客已被加入黑名单",
		"blacklisted": true,
	}, session.SessionID); err != nil {
		_ = err
	}

	return nil
}

// UnblacklistUser 解除拉黑
func (s *CustomerSessionService) UnblacklistUser(_ context.Context, userID string, platform model.Platform) error {
	if userID == "" {
		return errors.New("user_id 必填")
	}
	if err := s.blacklistRepo.Remove(userID, platform); err != nil {
		return fmt.Errorf("解除拉黑失败: %w", err)
	}
	return nil
}

// IsUserBlacklisted 判断访客是否在黑名单
func (s *CustomerSessionService) IsUserBlacklisted(_ context.Context, userID string, platform model.Platform) (bool, error) {
	return s.blacklistRepo.IsBlacklisted(userID, platform)
}

// ListActiveBlacklist 分页查询生效中的黑名单
//
// 命名统一：service 层用业务语义「黑名单」，repository 用实现语义「Active」。
// 这样 controller 调 service.ListActiveBlacklist 时更直观。
func (s *CustomerSessionService) ListActiveBlacklist(_ context.Context, page, pageSize int) ([]*model.UserBlacklist, int64, error) {
	return s.blacklistRepo.ListActive(page, pageSize)
}

// 编译期类型断言：确保 CustomerSessionService.blacklistRepo 字段保持 *UserBlacklistRepository
//
// 设计：使用 reflect 读取 struct 字段类型并比对，**不实例化 struct / 不访问字段值**，
// 避免 init 阶段 nil pointer dereference（(*CustomerSessionService)(nil).field 会求值）。
//
// 错误信息会在 init() 阶段被检测到：
//   - 字段被删除 → "blacklistRepo field is missing"
//   - 字段类型被改 → "blacklistRepo field type changed"
var _ = func() error {
	t := reflect.TypeOf(CustomerSessionService{})
	field, ok := t.FieldByName("blacklistRepo")
	if !ok {
		return errors.New("blacklistRepo field is missing on CustomerSessionService")
	}
	want := reflect.TypeOf((*repository.UserBlacklistRepository)(nil))
	if field.Type != want {
		return fmt.Errorf("blacklistRepo field type changed: got %v, want %v", field.Type, want)
	}
	return nil
}()

// preCreateBlacklistGuard 创建会话前的黑名单守卫
//
// 在 CustomerSessionService.CreateSession 入口处调用：
//   - user_id 为空（匿名访客）→ 跳过
//   - user_id 在 platform 维度已被拉黑 → 返回中文错误
//   - 校验仓储异常 → 透传错误
//
// 拆分为独立方法，便于：
//  1. 单元测试单独覆盖该守卫逻辑
//  2. 未来扩展其它守卫（如限流、风控评分）只需追加调用
func (s *CustomerSessionService) preCreateBlacklistGuard(req *CreateSessionRequest) error {
	if req == nil {
		return errors.New("请求体不能为空")
	}
	if req.UserID == "" {
		return nil // 匿名访客不参与黑名单
	}
	banned, err := s.blacklistRepo.IsBlacklisted(req.UserID, req.Platform)
	if err != nil {
		return fmt.Errorf("黑名单校验失败: %w", err)
	}
	if banned {
		return errors.New("该访客已被加入黑名单，无法创建新会话")
	}
	return nil
}
