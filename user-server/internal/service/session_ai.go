// session_ai.go AI 会话摘要（R48 T5，对标 Intercom Copilot / Libredesk 会话摘要）
package service

import (
	"context"
	"fmt"
	"strings"

	llm "hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// SessionAIService AI 会话服务
type SessionAIService struct {
	repo *repository.SessionAIRepo
}

// NewSessionAIService 构造
func NewSessionAIService(repo *repository.SessionAIRepo) *SessionAIService {
	return &SessionAIService{repo: repo}
}

// Generate 生成（或刷新）会话摘要：取会话消息→LLM LongSummary 场景→落库
func (s *SessionAIService) Generate(ctx context.Context, sessionID string) (*model.SessionAISummary, error) {
	msgs, err := s.repo.ListRecentMessages(ctx, sessionID, 60)
	if err != nil {
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
	if err := s.repo.UpsertSummary(ctx, rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// GetLatest 读取已有摘要（无则 ok=false）
func (s *SessionAIService) GetLatest(ctx context.Context, sessionID string) (*model.SessionAISummary, bool, error) {
	return s.repo.GetLatestSummary(ctx, sessionID)
}
