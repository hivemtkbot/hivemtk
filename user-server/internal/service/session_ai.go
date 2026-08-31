// session_ai.go AI 会话摘要（R48 T5，对标 Intercom Copilot / Libredesk 会话摘要）
package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	llm "hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// SessionAIService AI 会话服务
type SessionAIService struct{}

// NewSessionAIService 构造
func NewSessionAIService() *SessionAIService { return &SessionAIService{} }

// Generate 生成（或刷新）会话摘要：取会话消息→LLM LongSummary 场景→落库
func (s *SessionAIService) Generate(ctx context.Context, sessionID string) (*model.SessionAISummary, error) {
	g := db.GetDB()
	// 会话消息（最近 60 条，够概括且省 token）
	type msgRow struct {
		SenderType string
		SenderName string
		Content    string
		CreatedAt  time.Time
	}
	var msgs []msgRow
	if err := g.WithContext(ctx).
		Table("session_messages").
		Select("sender_type, COALESCE(sender_name,'') AS sender_name, content, created_at").
		Where("session_id = ? AND is_internal = ?", sessionID, false).
		Order("created_at ASC").Limit(60).Scan(&msgs).Error; err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, fmt.Errorf("会话无消息，无法生成摘要")
	}
	var sb strings.Builder
	for _, m := range msgs {
		who := m.SenderName
		if who == "" {
			switch m.SenderType {
			case "customer", "user":
				who = "客户"
			case "staff", "agent":
				who = "坐席"
			case "ai", "bot":
				who = "AI"
			default:
				who = m.SenderType
			}
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n", who, m.Content))
	}
	transcript := sb.String()
	r := []rune(transcript)
	if len(r) > 12000 {
		transcript = string(r[:12000])
	}

	// LLM 摘要：复用全局 Dispatcher（含 DB 路由/providers，勿自建实例绕过路由表）
	d := llm.GetGlobalDispatcher()
	res, err := d.Dispatch(ctx, llm.DispatchRequest{
		Scenario:    llm.ScenarioLongSummary,
		Prompt:      fmt.Sprintf("请为以下客服会话生成一段简洁摘要（150字以内），并在最后一行给出情绪倾向（格式：情绪: positive|neutral|negative）。\n\n会话记录：\n%s", transcript),
		MaxTokens:   400,
		Temperature: 0.3,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM 摘要失败: %w", err)
	}
	content := strings.TrimSpace(res.Content)
	sentiment := "neutral"
	for _, k := range []string{"positive", "negative", "neutral"} {
		if strings.Contains(strings.ToLower(content), "情绪: "+k) || strings.Contains(strings.ToLower(content), "情绪:"+k) {
			sentiment = k
		}
	}
	modelName := ""
	if res.Model != "" {
		modelName = res.Model
	}

	rec := &model.SessionAISummary{SessionID: sessionID, Summary: content, Sentiment: sentiment, Model: modelName}
	// upsert（一会话一摘要，重复生成覆盖）
	if err := g.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&model.SessionAISummary{}).Error; err != nil {
		return nil, err
	}
	if err := g.WithContext(ctx).Create(rec).Error; err != nil {
		return nil, err
	}
	return rec, nil
}

// GetLatest 读取已有摘要（无则 ok=false）
func (s *SessionAIService) GetLatest(ctx context.Context, sessionID string) (*model.SessionAISummary, bool, error) {
	var rec model.SessionAISummary
	err := db.GetDB().WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("id DESC").First(&rec).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &rec, true, nil
}
