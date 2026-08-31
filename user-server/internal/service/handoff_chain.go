package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// HandoffRule 升级规则定义
// 规则存储在 system_config_kv 中，key = "handoff_rules"，value 是 JSON 数组
type HandoffRule struct {
	ID          string `json:"id"`          // 规则唯一标识，如 "escalate_24h"
	Name        string `json:"name"`        // 展示名
	Enabled     bool   `json:"enabled"`     // 是否启用
	Order       int    `json:"order"`       // 执行优先级（从小到大）
	Condition   string `json:"condition"`   // 触发条件表达式，如 "unresolved > 24h" / "csat <= 2 && unresolved"
	TargetRole  string `json:"target_role"` // 目标角色: supervisor / specialist / agent
	Action      string `json:"action"`      // escalate / transfer / notify
	Description string `json:"description,omitempty"`
}

// HandoffChainService 工单升级/转派链服务
//
// G14: 竞品标配功能 - 基于可配置规则的 SLA 升级链
//
// 规则示例（默认）：
//  1. unresolved > 24h → 自动升级到 supervisor
//  2. csat <= 2 && unresolved → 自动转派到 specialist
//
// 数据源：
//   - system_config_kv (key = "handoff_rules")：规则 JSON
//   - handoff_decisions：已有的转人工决策记录（G14 在其基础上增加规则驱动的升级）
type HandoffChainService struct {
	db *gorm.DB
}

// NewHandoffChainService 创建转派链服务
func NewHandoffChainService() *HandoffChainService {
	return &HandoffChainService{db: db.GetDB()}
}

// NewHandoffChainServiceWithDB 注入 DB（测试用）
func (s *HandoffChainService) WithDB(d *gorm.DB) *HandoffChainService {
	s.db = d
	return s
}

// LoadRules 从 system_config_kv 加载规则
func (s *HandoffChainService) LoadRules(ctx context.Context) ([]HandoffRule, error) {
	if s.db == nil {
		return defaultHandoffRules(), nil
	}
	var kv model.SystemConfigKV
	// ⚠️ PostgreSQL 不支持 MySQL 风格反引号标识符，必须裸写 key（此处 key 在 PG 中为非保留字）。
	// 曾因 `` `key` `` 导致 syntax error at or near "=" ，使全部会话的升级规则判定失败并回退错误。
	err := s.db.WithContext(ctx).Where("key = ?", "handoff_rules").First(&kv).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return defaultHandoffRules(), nil
		}
		return nil, fmt.Errorf("load handoff_rules: %w", err)
	}
	var rules []HandoffRule
	if err := json.Unmarshal([]byte(kv.Value), &rules); err != nil {
		logger.Warnf("[HandoffChain] 解析 handoff_rules JSON 失败，使用默认规则: %v", err)
		return defaultHandoffRules(), nil
	}
	// 过滤禁用的
	out := make([]HandoffRule, 0, len(rules))
	for _, r := range rules {
		if r.Enabled {
			out = append(out, r)
		}
	}
	// 按 Order 排序
	sortHandoffRules(out)
	return out, nil
}

// SaveRules 保存规则到 system_config_kv
func (s *HandoffChainService) SaveRules(ctx context.Context, rules []HandoffRule) error {
	data, err := json.Marshal(rules)
	if err != nil {
		return fmt.Errorf("marshal rules: %w", err)
	}
	if s.db == nil {
		return fmt.Errorf("db 未初始化")
	}
	kv := model.SystemConfigKV{
		Key:   "handoff_rules",
		Value: string(data),
	}
	return s.db.WithContext(ctx).Save(&kv).Error
}

// CheckRules 对指定会话执行规则链检查
//
// 返回触发的规则列表（已执行）+ 建议动作
func (s *HandoffChainService) CheckRules(ctx context.Context, sessionID string) ([]string, error) {
	rules, err := s.LoadRules(ctx)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return nil, nil
	}
	if s.db == nil {
		return nil, nil
	}

	// 查会话状态
	type sessSnapshot struct {
		ID              string     `gorm:"column:id"`
		Status          string     `gorm:"column:status"`
		CreatedAt       time.Time  `gorm:"column:created_at"`
		ResolvedAt      *time.Time `gorm:"column:resolved_at"`
		CsatScore       *int       `gorm:"column:csat_score"`
		AssignedAgentID *uint      `gorm:"column:assigned_agent_id"`
	}
	var sess sessSnapshot
	if err := s.db.WithContext(ctx).
		Table("customer_sessions").
		Select("id, status, created_at, resolved_at, csat_score, assigned_agent_id").
		Where("id = ?", sessionID).
		Scan(&sess).Error; err != nil {
		return nil, fmt.Errorf("查询会话: %w", err)
	}

	now := time.Now()
	var triggered []string
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if s.matchRule(r, sess, now) {
			if err := s.applyRule(ctx, r, sessionID); err != nil {
				logger.Warnf("[HandoffChain] 规则 %s 应用失败: %v", r.ID, err)
				continue
			}
			triggered = append(triggered, r.ID)
			logger.Infof("[HandoffChain] 规则触发 rule=%s session=%s action=%s target=%s",
				r.ID, sessionID, r.Action, r.TargetRole)
		}
	}
	return triggered, nil
}

// RunCron 扫描全量未解决会话并执行规则链
// 供 cron 定期调用（建议每 5 分钟）
func (s *HandoffChainService) RunCron(ctx context.Context, limit int) (int, error) {
	if s.db == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 200
	}
	// 取 status 不是 resolved/closed 的会话
	type sessRow struct {
		ID string `gorm:"column:id"`
	}
	var rows []sessRow
	if err := s.db.WithContext(ctx).
		Table("customer_sessions").
		Select("id").
		Where("status NOT IN ?", []string{"resolved", "closed"}).
		Order("created_at ASC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return 0, err
	}
	triggeredTotal := 0
	for _, row := range rows {
		tr, err := s.CheckRules(ctx, row.ID)
		if err != nil {
			logger.Warnf("[HandoffChain] CheckRules session=%s 失败: %v", row.ID, err)
			continue
		}
		triggeredTotal += len(tr)
	}
	logger.Infof("[HandoffChain] cron 扫描完成 sessions=%d triggered=%d", len(rows), triggeredTotal)
	return triggeredTotal, nil
}

// matchRule 判断规则条件是否成立
func (s *HandoffChainService) matchRule(rule HandoffRule, sess struct {
	ID              string     `gorm:"column:id"`
	Status          string     `gorm:"column:status"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	ResolvedAt      *time.Time `gorm:"column:resolved_at"`
	CsatScore       *int       `gorm:"column:csat_score"`
	AssignedAgentID *uint      `gorm:"column:assigned_agent_id"`
}, now time.Time) bool {
	// 会话已解决 → 跳过
	if sess.Status == "resolved" || sess.Status == "closed" {
		return false
	}
	switch rule.ID {
	case "escalate_24h":
		// unresolved > 24h
		if sess.ResolvedAt != nil {
			return false
		}
		return now.Sub(sess.CreatedAt) > 24*time.Hour
	case "transfer_low_csat":
		// csat <= 2 && unresolved
		if sess.CsatScore == nil || *sess.CsatScore > 2 {
			return false
		}
		return sess.ResolvedAt == nil
	default:
		// 兜底：解析 condition 字符串做简单 key=value 匹配
		return evalSimpleCondition(rule.Condition, sess, now)
	}
}

// applyRule 执行规则动作
func (s *HandoffChainService) applyRule(ctx context.Context, rule HandoffRule, sessionID string) error {
	// 1) 写入 handoff_decisions 记录（规则驱动的升级/转派也落同一张表）
	if s.db != nil {
		record := model.HandoffDecisionRecord{
			DecisionID:   rule.ID + "_" + sessionID,
			SessionID:    sessionID,
			Reason:       rule.Action + "_by_rule:" + rule.ID,
			ReasonDetail: rule.Description,
			IntentType:   rule.TargetRole,
		}
		_ = s.db.WithContext(ctx).Create(&record).Error
	}
	// 2) 更新会话状态（根据 target_role）
	if s.db != nil && rule.Action == "escalate" {
		_ = s.db.WithContext(ctx).
			Table("customer_sessions").
			Where("id = ?", sessionID).
			Update("status", "escalated").Error
	}
	return nil
}

// evalSimpleCondition 简单条件求值（未匹配到预设 rule ID 时兜底）
func evalSimpleCondition(cond string, sess interface{}, now time.Time) bool {
	// 实现简化：仅做关键字包含匹配
	switch {
	case handoffContains(cond, "24h"):
		// 无法精确解析时保守返回 false（避免误触发）
		return false
	}
	return false
}

// handoffContains 避免与 customer_session_blacklist_test.go 中的 contains 重名
func handoffContains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && (len(sub) == 0 || handoffSearchSubstring(s, sub)))
}

func handoffSearchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// defaultHandoffRules 默认规则（首次使用时的种子数据）
func defaultHandoffRules() []HandoffRule {
	return []HandoffRule{
		{
			ID:          "escalate_24h",
			Name:        "24 小时未解决升级到主管",
			Enabled:     true,
			Order:       1,
			Condition:   "unresolved > 24h",
			TargetRole:  "supervisor",
			Action:      "escalate",
			Description: "会话创建超过 24 小时仍未解决，自动升级到 supervisor 处理",
		},
		{
			ID:          "transfer_low_csat",
			Name:        "低分+未解决转派专家",
			Enabled:     true,
			Order:       2,
			Condition:   "csat <= 2 && unresolved",
			TargetRole:  "specialist",
			Action:      "transfer",
			Description: "客户 CSAT 评分 ≤ 2 但会话仍未解决，自动转派到 specialist 跟进",
		},
	}
}

// sortHandoffRules 简单按 Order 升序排序
func sortHandoffRules(rules []HandoffRule) {
	for i := 1; i < len(rules); i++ {
		for j := i; j > 0 && rules[j-1].Order > rules[j].Order; j-- {
			rules[j-1], rules[j] = rules[j], rules[j-1]
		}
	}
}
