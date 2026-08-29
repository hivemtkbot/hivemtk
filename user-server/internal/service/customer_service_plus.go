// customer_service_plus.go 客服工作台增强（协作锁/内部备注/状态板/标签规则/快捷回复文件夹）
//
// 五层：本文件为 Service 层。数据访问全部经 Repository，不直连 DB。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// EditLockTTL 会话编辑锁 TTL（协作碰撞检测）
const EditLockTTL = 5 * time.Minute

// EditLock 会话编辑锁条目
type EditLock struct {
	SessionID string    `json:"session_id"`
	Holder    string    `json:"holder"`
	ExpiresAt time.Time `json:"expires_at"`
}

// CustomerServicePlusService 客服增强服务
type CustomerServicePlusService struct {
	sessionRepo *repository.CustomerSessionRepository
	msgRepo     *repository.SessionMessageRepository
	tagRepo     *repository.SessionTagRepository
	folderRepo  *repository.QuickReplyFolderRepository
	agentRepo   *repository.AgentStatusRepository

	mu    sync.Mutex
	locks map[string]EditLock
}

// NewCustomerServicePlusService 构造（DI 在路由装配完成）
func NewCustomerServicePlusService(
	sessionRepo *repository.CustomerSessionRepository,
	msgRepo *repository.SessionMessageRepository,
	agentRepo *repository.AgentStatusRepository,
) *CustomerServicePlusService {
	return &CustomerServicePlusService{
		sessionRepo: sessionRepo,
		msgRepo:     msgRepo,
		tagRepo:     repository.NewSessionTagRepository(),
		folderRepo:  repository.NewQuickReplyFolderRepository(),
		agentRepo:   agentRepo,
		locks:       map[string]EditLock{},
	}
}

// ---------- 编辑锁（多人协作碰撞检测） ----------

// AcquireEditLock 抢占编辑锁（他人持锁未过期 → false）
func (s *CustomerServicePlusService) AcquireEditLock(_ context.Context, sessionID, holder string) (EditLock, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if cur, ok := s.locks[sessionID]; ok && cur.ExpiresAt.After(now) && cur.Holder != holder {
		return cur, false
	}
	lock := EditLock{SessionID: sessionID, Holder: holder, ExpiresAt: now.Add(EditLockTTL)}
	s.locks[sessionID] = lock
	return lock, true
}

// ReleaseEditLock 释放锁（仅持有人可释放）
func (s *CustomerServicePlusService) ReleaseEditLock(_ context.Context, sessionID, holder string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.locks[sessionID]
	if !ok || (holder != "" && cur.Holder != holder) {
		return false
	}
	delete(s.locks, sessionID)
	return true
}

// GetEditLock 查询锁状态（过期视为无锁）
func (s *CustomerServicePlusService) GetEditLock(_ context.Context, sessionID string) (EditLock, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.locks[sessionID]
	if !ok || !cur.ExpiresAt.After(time.Now()) {
		return EditLock{}, false
	}
	return cur, true
}

// ---------- 内部备注（is_internal 消息，不发给客户） ----------

// AddInternalNote 写入内部备注
func (s *CustomerServicePlusService) AddInternalNote(ctx context.Context, sessionID, content, senderID, senderName string) (*model.SessionMessage, error) {
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("备注内容不能为空")
	}
	msg := &model.SessionMessage{
		SessionID:  sessionID,
		Content:    content,
		SenderType: "staff",
		SenderID:   senderID,
		SenderName: senderName,
		IsInternal: true,
	}
	if err := s.msgRepo.Create(ctx, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// ListInternalNotes 内部备注列表（仅 is_internal）
func (s *CustomerServicePlusService) ListInternalNotes(ctx context.Context, sessionID string, limit int) ([]*model.SessionMessage, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.msgRepo.ListInternalBySession(ctx, sessionID, limit)
}

// ---------- 标签规则 ----------

// SaveTagRuleRequest 规则保存请求
type TagRuleRequest struct {
	Code          string `json:"code" binding:"required"`
	RuleCondition string `json:"rule_condition"`
}

// ListTagRules 标签规则列表
func (s *CustomerServicePlusService) ListTagRules(ctx context.Context) ([]*model.SessionTag, error) {
	return s.tagRepo.GetByMerchant(ctx)
}

// SaveTagRule 保存规则条件（按 code 定位标签；不存在则自动创建；空条件=仅手动）
func (s *CustomerServicePlusService) SaveTagRule(ctx context.Context, req *TagRuleRequest) error {
	err := s.tagRepo.UpdateRuleCondition(ctx, req.Code, req.RuleCondition)
	if err == nil {
		return nil
	}
	// 标签不存在 → 自动创建后写入规则
	tag := &model.SessionTag{
		Name:          req.Code,
		Code:          req.Code,
		RuleCondition: req.RuleCondition,
	}
	if err := s.tagRepo.Create(ctx, tag); err != nil {
		return err
	}
	return nil
}

// ApplyTagRule 对会话执行标签规则：命中条件的 tag code 追加进 session.Tags
//
// 条件 JSON: {"keywords":["退款","投诉"]} — 命中会话任一消息即应用；空条件不应用。
func (s *CustomerServicePlusService) ApplyTagRule(ctx context.Context, sessionID string) (map[string]any, error) {
	sess, err := s.sessionRepo.GetBySessionID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	tags, err := s.tagRepo.GetByMerchant(ctx)
	if err != nil {
		return nil, err
	}
	msgs, err := s.msgRepo.ListAllBySessionID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	var text strings.Builder
	for _, m := range msgs {
		text.WriteString(m.Content)
		text.WriteString("\n")
	}
	current := parseSessionTags(sess.Tags)

	applied := []string{}
	for _, t := range tags {
		if t.RuleCondition == "" {
			continue
		}
		var cond struct {
			Keywords []string `json:"keywords"`
		}
		if err := json.Unmarshal([]byte(t.RuleCondition), &cond); err != nil {
			continue
		}
		if len(cond.Keywords) == 0 {
			continue
		}
		for _, kw := range cond.Keywords {
			if kw != "" && strings.Contains(text.String(), kw) {
				if !csPlusContainsString(current, t.Code) {
					current = append(current, t.Code)
					applied = append(applied, t.Code)
				}
				break
			}
		}
	}
	if len(applied) > 0 {
		merged, _ := json.Marshal(current)
		if err := s.sessionRepo.UpdateTags(ctx, sessionID, string(merged)); err != nil {
			return nil, err
		}
	}
	return map[string]any{"session_id": sessionID, "applied": applied, "tags": current}, nil
}

// ---------- 坐席状态板 ----------

// AgentBoardEntry 状态板行
type AgentBoardEntry struct {
	AgentID        uint       `json:"agent_id"`
	AgentName      string     `json:"agent_name"`
	Status         string     `json:"status"`
	OpenSessions   int        `json:"open_sessions"`
	MaxSessions    int        `json:"max_sessions"`
	TodayMessages  int        `json:"today_messages"`
	AvgResponseSec int        `json:"avg_response_sec"`
	LastActiveAt   *time.Time `json:"last_active_at,omitempty"`
}

// GetAgentStatusBoard 坐席状态板：全部坐席 + 在办会话（ActiveSessions 由分配/释放链路维护）
func (s *CustomerServicePlusService) GetAgentStatusBoard(ctx context.Context) ([]AgentBoardEntry, error) {
	agents, err := s.agentRepo.ListAllAgents(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AgentBoardEntry, 0, len(agents))
	for _, a := range agents {
		entry := AgentBoardEntry{
			AgentID:        a.AgentID,
			AgentName:      a.AgentName,
			Status:         a.Status,
			OpenSessions:   a.ActiveSessions,
			MaxSessions:    a.MaxSessions,
			TodayMessages:  a.TodayMessages,
			AvgResponseSec: a.AvgResponseTime,
		}
		if a.LastActiveAt != nil {
			t := *a.LastActiveAt
			entry.LastActiveAt = &t
		}
		out = append(out, entry)
	}
	return out, nil
}

// ---------- 快捷回复文件夹 ----------

// ListFolders 文件夹列表
func (s *CustomerServicePlusService) ListFolders(ctx context.Context) ([]*model.QuickReplyFolder, error) {
	return s.folderRepo.List(ctx)
}

// CreateFolder 创建文件夹（重名幂等返回现有）
func (s *CustomerServicePlusService) CreateFolder(ctx context.Context, name string) (*model.QuickReplyFolder, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("文件夹名不能为空")
	}
	return s.folderRepo.Create(ctx, name)
}

// ReorderFolder 调整文件夹顺序
func (s *CustomerServicePlusService) ReorderFolder(ctx context.Context, folderID uint, sortOrder int) error {
	return s.folderRepo.UpdateSortOrder(ctx, folderID, sortOrder)
}

// ---------- helpers ----------

func parseSessionTags(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		parts := strings.Split(raw, ",")
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				arr = append(arr, p)
			}
		}
	}
	if arr == nil {
		arr = []string{}
	}
	return arr
}

func csPlusContainsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// NewCustomerServicePlusServiceFromGlobal 便捷构造：使用全局 DB 仓储（控制器装配用）
func NewCustomerServicePlusServiceFromGlobal() *CustomerServicePlusService {
	return NewCustomerServicePlusService(
		repository.NewCustomerSessionRepository(),
		repository.NewSessionMessageRepository(),
		repository.NewAgentStatusRepository(),
	)
}

// DeleteFolder 删除文件夹（R43 补齐：可建可删）
func (s *CustomerServicePlusService) DeleteFolder(ctx context.Context, folderID uint) error {
	return s.folderRepo.Delete(ctx, folderID)
}
