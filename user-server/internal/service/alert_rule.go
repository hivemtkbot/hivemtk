package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// AlertRuleRequest 创建/更新规则请求
type AlertRuleRequest struct {
	Name           string   `json:"name" binding:"required,max=100"`
	Description    string   `json:"description" binding:"max=500"`
	Source         string   `json:"source" binding:"required"`
	Operator       string   `json:"operator" binding:"required"`
	Threshold      float64  `json:"threshold"`
	WindowSeconds  int      `json:"window_seconds"`
	CooldownSeconds int     `json:"cooldown_seconds"`
	Severity       string   `json:"severity"`
	Channels       []string `json:"channels"`
	Targets        map[string]any `json:"targets"`
	Enabled        *bool    `json:"enabled"`
}

// AlertRuleListItem 列表项
type AlertRuleListItem struct {
	model.AlertRule
	ChannelsList []string        `json:"channels_list"`
	TargetsMap   map[string]any  `json:"targets_map"`
}

// AlertRuleService 告警规则服务
type AlertRuleService struct {
	repo repository.AlertRuleRepository
	hist repository.AlertHistoryRepository
}

// NewAlertRuleService 构造
func NewAlertRuleService() *AlertRuleService {
	return &AlertRuleService{
		repo: repository.NewAlertRuleRepository(),
		hist: repository.NewAlertHistoryRepository(),
	}
}

var (
	ErrAlertRuleNotFound = errors.New("告警规则不存在")
	ErrAlertRuleInvalid  = errors.New("告警规则参数不合法")
)

var validOperators = map[string]bool{
	"gt": true, "ge": true, "lt": true, "le": true, "eq": true, "ne": true,
}

// validate 校验规则
func validateRule(req *AlertRuleRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("%w: name 必填", ErrAlertRuleInvalid)
	}
	if !validOperators[strings.ToLower(req.Operator)] {
		return fmt.Errorf("%w: operator 仅支持 gt/ge/lt/le/eq/ne", ErrAlertRuleInvalid)
	}
	if req.WindowSeconds <= 0 {
		req.WindowSeconds = 60
	}
	if req.CooldownSeconds < 0 {
		req.CooldownSeconds = 300
	}
	if req.Severity != string(model.AlertSeverityCritical) &&
		req.Severity != string(model.AlertSeverityInfo) {
		req.Severity = string(model.AlertSeverityWarning)
	}
	for _, c := range req.Channels {
		if !model.IsValidAlertChannel(c) {
			return fmt.Errorf("%w: 通知渠道 %s 不支持", ErrAlertRuleInvalid, c)
		}
	}
	return nil
}

// Create 创建规则
func (s *AlertRuleService) Create(ctx context.Context, req *AlertRuleRequest, creatorID uint) (*model.AlertRule, error) {
	if err := validateRule(req); err != nil {
		return nil, err
	}
	channels, _ := json.Marshal(req.Channels)
	targets, _ := json.Marshal(req.Targets)

	m := &model.AlertRule{
		Name:            strings.TrimSpace(req.Name),
		Description:      req.Description,
		Source:           strings.TrimSpace(req.Source),
		Operator:         strings.ToLower(req.Operator),
		Threshold:        req.Threshold,
		WindowSeconds:    req.WindowSeconds,
		CooldownSeconds:  req.CooldownSeconds,
		Severity:         model.AlertRuleSeverity(req.Severity),
		Channels:         string(channels),
		Targets:          string(targets),
		Enabled:          true,
		CreatedBy:        creatorID,
	}
	if req.Enabled != nil {
		m.Enabled = *req.Enabled
	}
	if err := s.repo.Create(ctx, m); err != nil {
		logger.Error(err, "创建告警规则失败")
		return nil, fmt.Errorf("创建告警规则失败: %w", err)
	}
	return m, nil
}

// Update 更新规则
func (s *AlertRuleService) Update(ctx context.Context, id uint, req *AlertRuleRequest) (*model.AlertRule, error) {
	if err := validateRule(req); err != nil {
		return nil, err
	}
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrAlertRuleNotFound
	}
	channels, _ := json.Marshal(req.Channels)
	targets, _ := json.Marshal(req.Targets)
	m.Name = strings.TrimSpace(req.Name)
	m.Description = req.Description
	m.Source = strings.TrimSpace(req.Source)
	m.Operator = strings.ToLower(req.Operator)
	m.Threshold = req.Threshold
	m.WindowSeconds = req.WindowSeconds
	m.CooldownSeconds = req.CooldownSeconds
	m.Severity = model.AlertRuleSeverity(req.Severity)
	m.Channels = string(channels)
	m.Targets = string(targets)
	if req.Enabled != nil {
		m.Enabled = *req.Enabled
	}
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, fmt.Errorf("更新告警规则失败: %w", err)
	}
	return m, nil
}

// Delete 删除规则
func (s *AlertRuleService) Delete(ctx context.Context, id uint) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAlertRuleNotFound
		}
		return err
	}
	return nil
}

// GetByID 查询单条
func (s *AlertRuleService) GetByID(ctx context.Context, id uint) (*model.AlertRule, error) {
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrAlertRuleNotFound
	}
	return m, nil
}

// List 列表
func (s *AlertRuleService) List(ctx context.Context, page, size int, enabledOnly bool) ([]*model.AlertRule, int64, error) {
	return s.repo.List(ctx, page, size, enabledOnly)
}

// SetStatus 批量启用/禁用
func (s *AlertRuleService) SetStatus(ctx context.Context, ids []uint, enabled bool) error {
	return s.repo.BatchUpdateStatus(ctx, ids, enabled)
}

// ListHistory 告警历史
func (s *AlertRuleService) ListHistory(ctx context.Context, page, size int, ruleID uint, source string) ([]*model.AlertHistory, int64, error) {
	return s.hist.List(ctx, page, size, ruleID, source)
}

// ResolveHistory 手动恢复告警历史
func (s *AlertRuleService) ResolveHistory(ctx context.Context, ruleID uint) error {
	return s.hist.ResolveFiring(ctx, ruleID, time.Now())
}

// UnreadAlerts 未恢复告警概览（OpsOverview 顶栏未读角标）
type UnreadAlerts struct {
	Count int64               `json:"count"`
	List  []*model.AlertHistory `json:"list"`
}

// GetUnread 返回未恢复（firing）告警计数与最近列表
func (s *AlertRuleService) GetUnread(ctx context.Context, limit int) (*UnreadAlerts, error) {
	count, err := s.hist.CountFiring(ctx)
	if err != nil {
		return nil, err
	}
	list, err := s.hist.ListFiring(ctx, limit)
	if err != nil {
		return nil, err
	}
	return &UnreadAlerts{Count: count, List: list}, nil
}
