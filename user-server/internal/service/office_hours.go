// office_hours.go 办公时间与离开自动回复（R48 T2，对标 Chatwoot/Intercom Business Hours）
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// OfficeHoursConfig 办公时间策略（KV: office.hours）
type OfficeHoursConfig struct {
	Enabled     bool        `json:"enabled"`
	DailyRanges [][2]string `json:"daily_ranges"` // [{start:"09:00", end:"18:00"}]
	AwayMessage string      `json:"away_message"`
}

// DefaultOfficeHoursConfig 默认工作日 9-18
func DefaultOfficeHoursConfig() OfficeHoursConfig {
	return OfficeHoursConfig{
		Enabled:     false,
		DailyRanges: [][2]string{{"09:00", "18:00"}},
		AwayMessage: "您好，当前为非工作时间，我们已在您的消息队列中记录，工作时间内将第一时间回复您。",
	}
}

const officeHoursKVKey = "office.hours"

// OfficeHoursService 办公时间服务
type OfficeHoursService struct {
	repo *repository.OfficeHoursRepo
}

// NewOfficeHoursService 构造
func NewOfficeHoursService(repo *repository.OfficeHoursRepo) *OfficeHoursService {
	return &OfficeHoursService{repo: repo}
}

// GetConfig 读策略（未配置回退默认）
func (s *OfficeHoursService) GetConfig(ctx context.Context) OfficeHoursConfig {
	cfg := DefaultOfficeHoursConfig()
	raw, err := repository.NewSystemConfigKVRepository().Get(ctx, officeHoursKVKey)
	if err != nil || raw == "" {
		return cfg
	}
	var parsed OfficeHoursConfig
	if json.Unmarshal([]byte(raw), &parsed) == nil {
		if len(parsed.DailyRanges) == 0 {
			parsed.DailyRanges = cfg.DailyRanges
		}
		if strings.TrimSpace(parsed.AwayMessage) == "" {
			parsed.AwayMessage = cfg.AwayMessage
		}
		return parsed
	}
	return cfg
}

// SaveConfig 保存策略（校验时间格式）
func (s *OfficeHoursService) SaveConfig(ctx context.Context, cfg *OfficeHoursConfig) error {
	for _, r := range cfg.DailyRanges {
		for _, t := range r {
			if len(t) != 5 || t[2] != ':' {
				return fmt.Errorf("时间格式必须为 HH:MM，收到 %q", t)
			}
		}
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = repository.NewSystemConfigKVRepository().Upsert(ctx, officeHoursKVKey, string(raw))
	return err
}

// IsWithinOfficeHours 当前是否在办公时间内（未启用=恒 true）
func (s *OfficeHoursService) IsWithinOfficeHours(ctx context.Context) bool {
	cfg := s.GetConfig(ctx)
	if !cfg.Enabled || len(cfg.DailyRanges) == 0 {
		return true
	}
	now := time.Now()
	cur := fmt.Sprintf("%02d:%02d", now.Hour(), now.Minute())
	for _, r := range cfg.DailyRanges {
		if cur >= r[0] && cur <= r[1] {
			return true
		}
	}
	return false
}

// AwayMessageFor 返回离开提示语（工作时间内返回空串）
func (s *OfficeHoursService) AwayMessageFor(ctx context.Context) string {
	if s.IsWithinOfficeHours(ctx) {
		return ""
	}
	cfg := s.GetConfig(ctx)
	return cfg.AwayMessage
}

// SendAwayReplyIfClosed 新会话离开时间自动回复（防循环：origin 标记 system_away 的消息不再触发）
// 返回 true = 已入队 away 回复
func (s *OfficeHoursService) SendAwayReplyIfClosed(ctx context.Context, sessionID, conversationID, platform, accountID string) bool {
	msg := s.AwayMessageFor(ctx)
	if msg == "" {
		return false
	}
	cnt, err := s.repo.CountAwayReplyRecent(ctx, conversationID, 2*time.Hour)
	if err != nil || cnt > 0 {
		return false
	}
	now := time.Now()
	rec := &model.MessageHub{
		Platform:       platform,
		MsgID:          fmt.Sprintf("away_%s_%d", sessionID, now.Unix()),
		AccountID:      accountID,
		Direction:      "outbound",
		Status:         "pending",
		MsgType:        "text",
		SenderID:       "system",
		SenderName:     "离开自动回复",
		Content:        msg,
		ConversationID: conversationID,
		TraceID:        "away",
		SentAt:         now,
	}
	return s.repo.CreateMessageHub(ctx, rec) == nil
}
