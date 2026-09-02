// session_chain.go R53 会话生命周期链闭环（对标 Chatwoot 链2/链3）
//
// A1 resolved/closed → CSAT 自动触发（csat_survey_listener 语义）
// A2 自动解决 SLA：auto_resolve_hours 无活动超时 → 打标+关闭（resolution_job 语义）
// A3 访客消息 → resolved/closed 会话自动 reopen（toggle_status 语义）
// B  轻量自动化规则引擎（automation_rules 精简版：事件+条件+动作+延迟）
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// SessionChainService 会话生命周期链服务
type SessionChainService struct {
	db *gorm.DB
}

// NewSessionChainService 构造
func NewSessionChainService(gdb *gorm.DB) *SessionChainService { return &SessionChainService{db: gdb} }

// NewSessionChainServiceFromGlobal 便捷构造
func NewSessionChainServiceFromGlobal() *SessionChainService { return NewSessionChainService(db.GetDB()) }

// ---------- A1: resolved/closed 自动触发 CSAT ----------

// TriggerCSATOnClose 会话关闭时自动下发 CSAT（csat_survey_listener 语义）。
// 由 UpdateSessionStatus 在 resolved/closed 迁移点调用；fire-and-forget 不阻塞。
func (s *SessionChainService) TriggerCSATOnClose(session *model.CustomerSession) {
	if session == nil || session.SessionID == "" {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[panic-recover] %T: %v\n%s", r, r, string(debug.Stack()))
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), utils.RagMetricsTimeout)
		defer cancel()
		csat := NewCSATService()
		if _, err := csat.Trigger(ctx, session.SessionID, "auto"); err != nil {
			// 已有调查（幂等）或会话缺失均静默
			return
		}
	}()
}

// ---------- A2: 自动解决 SLA ----------

// AutoResolveConfigKey KV 键
const AutoResolveConfigKey = "session.auto_resolve"

// AutoResolveConfig SLA 配置
type AutoResolveConfig struct {
	Enabled       bool   `json:"enabled"`
	Hours         int    `json:"hours"`          // 无活动超过 N 小时自动解决
	OnlyClosedOff bool   `json:"only_open_like"` // 仅 open 类状态（pending/ai_handling/waiting）
	AddTag        string `json:"add_tag"`        // 自动关闭时打的标签（如 auto_resolved）
}

// DefaultAutoResolveConfig 默认关闭
func DefaultAutoResolveConfig() AutoResolveConfig {
	return AutoResolveConfig{Enabled: false, Hours: 72, OnlyClosedOff: true, AddTag: "auto_resolved"}
}

// GetAutoResolveConfig 读配置
func (s *SessionChainService) GetAutoResolveConfig(ctx context.Context) AutoResolveConfig {
	cfg := DefaultAutoResolveConfig()
	raw, err := repository.NewSystemConfigKVRepository().Get(ctx, AutoResolveConfigKey)
	if err != nil || raw == "" {
		return cfg
	}
	var p AutoResolveConfig
	if json.Unmarshal([]byte(raw), &p) == nil {
		if p.Hours <= 0 {
			p.Hours = cfg.Hours
		}
		return p
	}
	return cfg
}

// SaveAutoResolveConfig 存配置
func (s *SessionChainService) SaveAutoResolveConfig(ctx context.Context, cfg *AutoResolveConfig) error {
	if cfg.Hours <= 0 || cfg.Hours > 24*90 {
		return fmt.Errorf("hours 必须在 1-2160")
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = repository.NewSystemConfigKVRepository().Upsert(ctx, AutoResolveConfigKey, string(raw))
	return err
}

// RunAutoResolve SLA 扫描（cron 入口）：无活动超时会话 → 打标 + 关闭
func (s *SessionChainService) RunAutoResolve(ctx context.Context) (int, error) {
	cfg := s.GetAutoResolveConfig(ctx)
	if !cfg.Enabled {
		return 0, nil
	}
	g := s.db
	threshold := time.Now().Add(-time.Duration(cfg.Hours) * time.Hour)
	var sessions []*model.CustomerSession
	q := g.WithContext(ctx).
		Where("status IN ? AND updated_at < ?", []model.SessionStatus{
			model.SessionStatusPending, model.SessionStatusAIHandling, model.SessionStatusWaiting,
		}, threshold).
		Limit(200).
		Find(&sessions)
	if q.Error != nil {
		return 0, q.Error
	}
	closed := 0
	for _, sess := range sessions {
		// 打标
		if cfg.AddTag != "" {
			var tags []string
			_ = json.Unmarshal([]byte(sess.Tags), &tags)
			has := false
			for _, t := range tags {
				if t == cfg.AddTag {
					has = true
					break
				}
			}
			if !has {
				tags = append(tags, cfg.AddTag)
				if merged, err := json.Marshal(tags); err == nil {
					_ = g.WithContext(ctx).Model(&model.CustomerSession{}).
						Where("id = ?", sess.ID).Update("tags", string(merged)).Error
				}
			}
		}
		if err := s.closeByPK(ctx, sess.ID); err == nil {
			closed++
		}
	}
	return closed, nil
}

// closeByPK 直接按主键关单（复用 UpdateStatus 需要再查一次，这里省一轮）
func (s *SessionChainService) closeByPK(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Model(&model.CustomerSession{}).
		Where("id = ?", id).Update("status", model.SessionStatusClosed).Error
}

// ---------- A3: 访客消息 → resolved/closed 自动 reopen ----------

// ReopenOnInboundMessage 访客消息落库后调用：resolved/closed 会话自动回 waiting（toggle_status 语义）。
// 返回 true=发生了 reopen。
func (s *SessionChainService) ReopenOnInboundMessage(ctx context.Context, sessionID string) (bool, error) {
	g := s.db
	res := g.WithContext(ctx).
		Model(&model.CustomerSession{}).
		Where("session_id = ? AND status IN ?", sessionID, []model.SessionStatus{
			model.SessionStatusResolved, model.SessionStatusClosed,
		}).
		Update("status", model.SessionStatusWaiting)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// GetSession 根据 session_id 获取会话（供 controller 层复用，避免直接 db.GetDB）
func (s *SessionChainService) GetSession(ctx context.Context, sessionID string) (*model.CustomerSession, error) {
	repo := repository.NewCustomerSessionRepositoryWithDB(s.db)
	return repo.GetBySessionID(ctx, sessionID)
}

// ---------- B: 轻量自动化规则引擎 ----------

// 支持的事件
const (
	RuleEventConversationCreated = "conversation_created"
	RuleEventMessageInbound      = "message_inbound"
	RuleEventSessionResolved     = "session_resolved"
)

// 支持的动作
const (
	RuleActAddTag      = "add_tag"
	RuleActSetPriority = "set_priority"
	RuleActAssign      = "assign"
	RuleActClose       = "close"
	RuleActSendMessage = "send_message"
	RuleActAddNote     = "add_note"
	RuleActWebhook     = "webhook"
)

// RuleCondition 条件
type RuleCondition struct {
	Field string `json:"field"` // platform/status/one_id/priority/tags/content
	Op    string `json:"op"`    // eq/contains/gt/lt
	Value string `json:"value"`
}

// RuleAction 动作
type RuleAction struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// RuleEngineService 规则引擎
type RuleEngineService struct {
	db     *gorm.DB
	csPlus *CustomerServicePlusService
	now    func() time.Time
}

// NewRuleEngineService 构造
func NewRuleEngineService(gdb *gorm.DB) *RuleEngineService {
	return &RuleEngineService{db: gdb, csPlus: NewCustomerServicePlusServiceFromGlobal(), now: time.Now}
}

// NewRuleEngineServiceFromGlobal 便捷构造
func NewRuleEngineServiceFromGlobal() *RuleEngineService { return NewRuleEngineService(db.GetDB()) }

// Create 创建规则
func (s *RuleEngineService) Create(ctx context.Context, r *model.AutomationRule) (*model.AutomationRule, error) {
	if r.Event != RuleEventConversationCreated && r.Event != RuleEventMessageInbound && r.Event != RuleEventSessionResolved {
		return nil, fmt.Errorf("不支持的事件: %s", r.Event)
	}
	var conds []RuleCondition
	if err := json.Unmarshal([]byte(r.Conditions), &conds); err != nil {
		return nil, fmt.Errorf("conditions JSON 非法")
	}
	for _, c := range conds {
		switch c.Op {
		case "eq", "contains", "gt", "lt":
		default:
			return nil, fmt.Errorf("不支持的条件操作符: %s", c.Op)
		}
	}
	var acts []RuleAction
	if err := json.Unmarshal([]byte(r.Actions), &acts); err != nil || len(acts) == 0 {
		return nil, fmt.Errorf("actions JSON 非法或为空")
	}
	for _, a := range acts {
		switch a.Type {
		case RuleActAddTag, RuleActSetPriority, RuleActAssign, RuleActClose, RuleActSendMessage, RuleActAddNote, RuleActWebhook:
		default:
			return nil, fmt.Errorf("不支持的动作: %s", a.Type)
		}
	}
	if err := s.db.WithContext(ctx).Create(r).Error; err != nil {
		return nil, err
	}
	return r, nil
}

// List 规则列表
func (s *RuleEngineService) List(ctx context.Context, event string) ([]*model.AutomationRule, error) {
	q := s.db.WithContext(ctx).Model(&model.AutomationRule{})
	if event != "" {
		q = q.Where("event = ?", event)
	}
	var list []*model.AutomationRule
	err := q.Order("priority ASC, id ASC").Find(&list).Error
	return list, err
}

// Delete 删除
func (s *RuleEngineService) Delete(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Delete(&model.AutomationRule{}, id).Error
}

// Toggle 启停
func (s *RuleEngineService) Toggle(ctx context.Context, id uint, enabled bool) error {
	return s.db.WithContext(ctx).Model(&model.AutomationRule{}).
		Where("id = ?", id).Update("enabled", enabled).Error
}

// DispatchSessionEvent 会话事件入口（session 落库/迁移点调用）
func (s *RuleEngineService) DispatchSessionEvent(ctx context.Context, event, sessionID string, session *model.CustomerSession) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[panic-recover] %T: %v\n%s", r, r, string(debug.Stack()))
			}
		}()
		c, cancel := context.WithTimeout(context.Background(), utils.DefaultHTTPTimeout)
		defer cancel()
		s.DispatchWithText(c, event, sessionID, "", session)
	}()
}

// Dispatch 规则匹配执行（inboundText: message_inbound 事件的消息内容，供 content 条件匹配）
func (s *RuleEngineService) Dispatch(ctx context.Context, event, sessionID string, session *model.CustomerSession) {
	s.DispatchWithText(ctx, event, sessionID, "", session)
}

// DispatchWithText 完整入口（带消息内容）
func (s *RuleEngineService) DispatchWithText(ctx context.Context, event, sessionID string, inboundText string, session *model.CustomerSession) {
	g := s.db
	var rules []*model.AutomationRule
	if err := g.WithContext(ctx).
		Where("event = ? AND enabled = ?", event, true).
		Order("priority ASC, id ASC").Limit(20).
		Find(&rules).Error; err != nil {
		return
	}
	for _, rule := range rules {
		if session == nil || !s.matchConditions(rule, session, inboundText) {
			continue
		}
		if rule.DelayMinutes > 0 {
			pending := &model.RulePendingExecution{
				RuleID: rule.ID, SessionID: sessionID,
				ExecuteAt: s.now().Add(time.Duration(rule.DelayMinutes) * time.Minute),
			}
			_ = g.WithContext(ctx).Create(pending).Error
			continue
		}
		s.executeRule(ctx, rule, sessionID, session)
	}
}

// matchConditions 全条件满足判定
func (s *RuleEngineService) matchConditions(rule *model.AutomationRule, sess *model.CustomerSession, inboundText string) bool {
	var conds []RuleCondition
	if json.Unmarshal([]byte(rule.Conditions), &conds) != nil {
		return true // 条件非法视为无条件（宽松匹配）
	}
	for _, c := range conds {
		var actual string
		switch c.Field {
		case "platform":
			actual = string(sess.Platform)
		case "status":
			actual = string(sess.Status)
		case "one_id":
			actual = sess.OneID
		case "account_id":
			actual = sess.AccountID
		case "priority":
			actual = fmt.Sprintf("%d", sess.Priority)
		case "tags":
			actual = sess.Tags
		case "content":
			actual = inboundText
		default:
			return false
		}
		switch c.Op {
		case "eq":
			if actual != c.Value {
				return false
			}
		case "contains":
			if !strings.Contains(actual, c.Value) {
				return false
			}
		case "gt":
			if !(actual > c.Value) {
				return false
			}
		case "lt":
			if !(actual < c.Value) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// executeRule 执行动作序列
func (s *RuleEngineService) executeRule(ctx context.Context, rule *model.AutomationRule, sessionID string, sess *model.CustomerSession) {
	var acts []RuleAction
	if json.Unmarshal([]byte(rule.Actions), &acts) != nil {
		return
	}
	g := s.db
	for _, a := range acts {
		var err error
		switch a.Type {
		case RuleActAddTag:
			var tags []string
			_ = json.Unmarshal([]byte(sess.Tags), &tags)
			has := false
			for _, t := range tags {
				if t == a.Value {
					has = true
					break
				}
			}
			if !has {
				tags = append(tags, a.Value)
				if merged, jm := json.Marshal(tags); jm == nil {
					err = g.WithContext(ctx).Model(&model.CustomerSession{}).
						Where("session_id = ?", sessionID).Update("tags", string(merged)).Error
				}
			}
		case RuleActSetPriority:
			lvl := 0
			fmt.Sscanf(a.Value, "%d", &lvl)
			err = g.WithContext(ctx).Model(&model.CustomerSession{}).
				Where("session_id = ?", sessionID).Update("priority", lvl).Error
		case RuleActAssign:
			err = g.WithContext(ctx).Model(&model.CustomerSession{}).
				Where("session_id = ?", sessionID).Update("agent_id", a.Value).Error
		case RuleActClose:
			err = g.WithContext(ctx).Model(&model.CustomerSession{}).
				Where("session_id = ?", sessionID).Update("status", model.SessionStatusClosed).Error
		case RuleActSendMessage:
			now := time.Now()
			rec := &model.MessageHub{
				Platform:       string(sess.Platform),
				MsgID:          fmt.Sprintf("rule_%s_%s_%d", rule.Name, sessionID, now.UnixNano()),
				AccountID:      sess.AccountID,
				Direction:      "outbound",
				Status:         "pending",
				MsgType:        "text",
				SenderID:       "system",
				SenderName:     "自动化",
				Content:        a.Value,
				ConversationID: sessionID,
				TraceID:        "rule",
				SentAt:         now,
			}
			err = g.WithContext(ctx).Create(rec).Error
		case RuleActAddNote:
			m := NewSessionMessageRepository()
			err = m.Create(ctx, &model.SessionMessage{
				SessionID: sessionID, Content: fmt.Sprintf("[自动化:%s] %s", rule.Name, a.Value),
				SenderType: "staff", SenderName: "自动化", IsInternal: true,
			})
		case RuleActWebhook:
			PublishWebhookEvent(ctx, "rule.triggered", map[string]any{
				"rule": rule.Name, "session_id": sessionID, "action": a.Type, "value": a.Value,
			})
		}
		if err != nil {
			slog.Warn("[RuleEngine] 动作执行失败", "rule", rule.Name, "action", a.Type, "err", err)
			return
		}
	}
	_ = g.WithContext(ctx).Model(rule).UpdateColumn("run_count", gorm.Expr("run_count + 1")).Error
}

// ProcessPendingRules 延迟规则复核（cron 入口）
func (s *RuleEngineService) ProcessPendingRules(ctx context.Context) (int, error) {
	g := s.db
	var pendings []*model.RulePendingExecution
	if err := g.WithContext(ctx).
		Where("status = ? AND execute_at <= ?", "pending", time.Now()).
		Order("execute_at ASC").Limit(50).Find(&pendings).Error; err != nil {
		return 0, err
	}
	done := 0
	for _, p := range pendings {
		var rule model.AutomationRule
		if err := g.WithContext(ctx).First(&rule, p.RuleID).Error; err != nil || !rule.Enabled {
			_ = g.WithContext(ctx).Model(p).Update("status", "failed").Error
			continue
		}
		var sess model.CustomerSession
		if err := g.WithContext(ctx).Where("session_id = ?", p.SessionID).First(&sess).Error; err != nil {
			_ = g.WithContext(ctx).Model(p).Update("status", "failed").Error
			continue
		}
		if !s.matchConditions(&rule, &sess, "") {
			_ = g.WithContext(ctx).Model(p).Update("status", "done").Error
			done++
			continue
		}
		s.executeRule(ctx, &rule, p.SessionID, &sess)
		_ = g.WithContext(ctx).Model(p).Update("status", "done").Error
		done++
	}
	return done, nil
}

// SessionMessageRepoAlias 别名（executeRule AddNote 用）
func NewSessionMessageRepository() *repository.SessionMessageRepository {
	return repository.NewSessionMessageRepository()
}
