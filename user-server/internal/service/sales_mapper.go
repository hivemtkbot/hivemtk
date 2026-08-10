package service

import (
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
)

// sales_mapper.go 销售链路 model ↔ dto 转换
// 转换属业务层职责（P0-7：dto 层不再引用 model，转换统一收口在 service）

// LLMProviderConfigToDTO model 嵌入式配置 → dto 镜像
func LLMProviderConfigToDTO(c model.LLMProviderConfig) dto.LLMProviderConfig {
	return dto.LLMProviderConfig{
		APIKey:         c.APIKey,
		BaseURL:        c.BaseURL,
		APIType:        c.APIType,
		Model:          c.Model,
		MaxRetries:     c.MaxRetries,
		RequestTimeout: c.RequestTimeout,
	}
}

// DialogueMemoryToDTO 对话记忆实体 → dto 视图
func DialogueMemoryToDTO(m *model.DialogueMemory) *dto.DialogueMemory {
	if m == nil {
		return nil
	}
	return &dto.DialogueMemory{
		ID:                   m.ID,
		SessionID:            m.SessionID,
		CustomerID:           m.CustomerID,
		Summary:              m.Summary,
		KeyFacts:             map[string]any(m.KeyFacts),
		CustomerName:         m.CustomerName,
		CustomerPhone:        m.CustomerPhone,
		CustomerWechat:       m.CustomerWechat,
		Budget:               m.Budget,
		Demand:               m.Demand,
		Objections:           []any(m.Objections),
		PurchaseIntent:       m.PurchaseIntent,
		IntentTrail:          []any(m.IntentTrail),
		SOPHistory:           []any(m.SOPHistory),
		LastAction:           m.LastAction,
		NextActionSuggestion: m.NextActionSuggestion,
		LastActiveAt:         m.LastActiveAt,
		MessageCount:         m.MessageCount,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
	}
}

// SOPAgentToDTO SOP 智能体实体 → dto 视图
func SOPAgentToDTO(s *model.SOPAgent) *dto.SOPAgent {
	if s == nil {
		return nil
	}
	return &dto.SOPAgent{
		ID:             s.ID,
		Name:           s.Name,
		Scenario:       s.Scenario,
		Description:    s.Description,
		TriggerType:    s.TriggerType,
		TriggerConfig:  map[string]any(s.TriggerConfig),
		SOPGraph:       map[string]any(s.SOPGraph),
		Version:        s.Version,
		IsActive:       s.IsActive,
		Priority:       s.Priority,
		ExecutionCount: s.ExecutionCount,
		SuccessCount:   s.SuccessCount,
		ABTestConfig:   map[string]any(s.ABTestConfig),
		UseBandit:      s.UseBandit,
		CreatedBy:      s.CreatedBy,
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
	}
}

// RichCardsToDTO model 富卡片切片 → dto 镜像
func RichCardsToDTO(cards []model.RichCard) []dto.RichCard {
	if cards == nil {
		return nil
	}
	out := make([]dto.RichCard, 0, len(cards))
	for _, c := range cards {
		out = append(out, richCardToDTO(c))
	}
	return out
}

// RichCardsFromDTO dto 富卡片切片 → model（出站/落库链路使用）
func RichCardsFromDTO(cards []dto.RichCard) []model.RichCard {
	if cards == nil {
		return nil
	}
	out := make([]model.RichCard, 0, len(cards))
	for _, c := range cards {
		mc := model.RichCard{
			Type:        model.RichCardType(c.Type),
			Title:       c.Title,
			Subtitle:    c.Subtitle,
			Description: c.Description,
			ImageURL:    c.ImageURL,
			ThumbURL:    c.ThumbURL,
			Fields:      c.Fields,
		}
		for _, b := range c.Buttons {
			mc.Buttons = append(mc.Buttons, model.CardButton{Text: b.Text, URL: b.URL, Action: b.Action})
		}
		out = append(out, mc)
	}
	return out
}

func richCardToDTO(c model.RichCard) dto.RichCard {
	dc := dto.RichCard{
		Type:        dto.RichCardType(c.Type),
		Title:       c.Title,
		Subtitle:    c.Subtitle,
		Description: c.Description,
		ImageURL:    c.ImageURL,
		ThumbURL:    c.ThumbURL,
		Fields:      c.Fields,
	}
	for _, b := range c.Buttons {
		dc.Buttons = append(dc.Buttons, dto.CardButton{Text: b.Text, URL: b.URL, Action: b.Action})
	}
	return dc
}
