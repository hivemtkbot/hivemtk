package model

import (
	"testing"
)

func TestAIGenerationType_Constants(t *testing.T) {
	if AIGenerationTypeCopywriting != "copywriting" {
		t.Errorf("Expected AIGenerationTypeCopywriting 'copywriting', got %s", AIGenerationTypeCopywriting)
	}
	if AIGenerationTypeTitle != "title" {
		t.Errorf("Expected AIGenerationTypeTitle 'title', got %s", AIGenerationTypeTitle)
	}
	if AIGenerationTypeSummary != "summary" {
		t.Errorf("Expected AIGenerationTypeSummary 'summary', got %s", AIGenerationTypeSummary)
	}
	if AIGenerationTypeReply != "reply" {
		t.Errorf("Expected AIGenerationTypeReply 'reply', got %s", AIGenerationTypeReply)
	}
	if AIGenerationTypeTranslation != "translation" {
		t.Errorf("Expected AIGenerationTypeTranslation 'translation', got %s", AIGenerationTypeTranslation)
	}
	if AIGenerationTypeRewrite != "rewrite" {
		t.Errorf("Expected AIGenerationTypeRewrite 'rewrite', got %s", AIGenerationTypeRewrite)
	}
	if AIGenerationTypeExpand != "expand" {
		t.Errorf("Expected AIGenerationTypeExpand 'expand', got %s", AIGenerationTypeExpand)
	}
	if AIGenerationTypePolish != "polish" {
		t.Errorf("Expected AIGenerationTypePolish 'polish', got %s", AIGenerationTypePolish)
	}
	if AIGenerationTypeKeywords != "keywords" {
		t.Errorf("Expected AIGenerationTypeKeywords 'keywords', got %s", AIGenerationTypeKeywords)
	}
	if AIGenerationTypeDescription != "description" {
		t.Errorf("Expected AIGenerationTypeDescription 'description', got %s", AIGenerationTypeDescription)
	}
	if AIGenerationTypeAdCopy != "ad_copy" {
		t.Errorf("Expected AIGenerationTypeAdCopy 'ad_copy', got %s", AIGenerationTypeAdCopy)
	}
	if AIGenerationTypeSocialPost != "social_post" {
		t.Errorf("Expected AIGenerationTypeSocialPost 'social_post', got %s", AIGenerationTypeSocialPost)
	}
	if AIGenerationTypeEmail != "email" {
		t.Errorf("Expected AIGenerationTypeEmail 'email', got %s", AIGenerationTypeEmail)
	}
	if AIGenerationTypeScript != "script" {
		t.Errorf("Expected AIGenerationTypeScript 'script', got %s", AIGenerationTypeScript)
	}
}

func TestAIGenerationRecord_TableName(t *testing.T) {
	record := &AIGenerationRecord{}
	tableName := record.TableName()
	if tableName != "ai_generation_records" {
		t.Errorf("Expected table name 'ai_generation_records', got %s", tableName)
	}
}

func TestAIGenerationRecord_BasicFields(t *testing.T) {
	record := &AIGenerationRecord{
		ID:         1,
		UserID:     100,
		Type:       AIGenerationTypeCopywriting,
		Input:      "写一篇关于 AI 的文章",
		Output:     "AI 是人工智能的缩写...",
		TemplateID: 5,
		Model:      "gpt-4",
		TokensUsed: 500,
		IsSaved:    true,
		IsFavorite: false,
		Rating:     5,
	}

	if record.ID != 1 {
		t.Errorf("Expected ID 1, got %d", record.ID)
	}

	if record.UserID != 100 {
		t.Errorf("Expected UserID 100, got %d", record.UserID)
	}
	if record.Type != AIGenerationTypeCopywriting {
		t.Errorf("Expected Type 'copywriting', got %s", record.Type)
	}
	if record.Input != "写一篇关于 AI 的文章" {
		t.Errorf("Expected Input, got %s", record.Input)
	}
	if record.Output != "AI 是人工智能的缩写..." {
		t.Errorf("Expected Output, got %s", record.Output)
	}
	if record.TemplateID != 5 {
		t.Errorf("Expected TemplateID 5, got %d", record.TemplateID)
	}
	if record.Model != "gpt-4" {
		t.Errorf("Expected Model 'gpt-4', got %s", record.Model)
	}
	if record.TokensUsed != 500 {
		t.Errorf("Expected TokensUsed 500, got %d", record.TokensUsed)
	}
	if !record.IsSaved {
		t.Error("Expected IsSaved to be true")
	}
	if record.IsFavorite {
		t.Error("Expected IsFavorite to be false")
	}
	if record.Rating != 5 {
		t.Errorf("Expected Rating 5, got %d", record.Rating)
	}
}

func TestAIGenerationRecord_DefaultValues(t *testing.T) {
	record := &AIGenerationRecord{}

	if record.IsSaved != false {
		t.Logf("IsSaved is %v (expected false before save, default is false)", record.IsSaved)
	}
	if record.IsFavorite != false {
		t.Logf("IsFavorite is %v (expected false before save, default is false)", record.IsFavorite)
	}
}

func TestAIGenerationRecord_WithRatings(t *testing.T) {
	ratings := []int{1, 2, 3, 4, 5}

	for _, rating := range ratings {
		record := &AIGenerationRecord{
			Rating: rating,
		}
		if record.Rating != rating {
			t.Errorf("Expected Rating %d, got %d", rating, record.Rating)
		}
	}
}

func TestAIGenerationRecord_WithTypes(t *testing.T) {
	types := []AIGenerationType{
		AIGenerationTypeCopywriting,
		AIGenerationTypeTitle,
		AIGenerationTypeReply,
		AIGenerationTypeTranslation,
		AIGenerationTypeScript,
	}

	for _, aiType := range types {
		record := &AIGenerationRecord{
			Type: aiType,
		}
		if record.Type != aiType {
			t.Errorf("Expected Type %s, got %s", aiType, record.Type)
		}
	}
}

func TestAIGenerationRecord_WithLongInput(t *testing.T) {
	longInput := "这是一篇很长的输入内容，包含详细的背景信息、要求和期望输出格式。AI 需要根据这些输入生成符合要求的文案内容。"
	record := &AIGenerationRecord{
		Input: longInput,
	}

	if record.Input != longInput {
		t.Error("Expected long input to be stored")
	}
}

func TestPromptTemplate_TableName(t *testing.T) {
	template := &PromptTemplate{}
	tableName := template.TableName()
	if tableName != "prompt_templates" {
		t.Errorf("Expected table name 'prompt_templates', got %s", tableName)
	}
}

func TestPromptTemplate_BasicFields(t *testing.T) {
	template := &PromptTemplate{
		ID:          1,
		Name:        "文案生成模板",
		Type:        AIGenerationTypeCopywriting,
		Template:    "请为{{product}}生成一篇营销文案，目标用户是{{target_audience}}",
		Variables:   `{"product": "产品名称", "target_audience": "目标用户"}`,
		Description: "用于生成产品营销文案的模板",
		Example:     "这款产品设计精美，功能强大，非常适合年轻人使用。",
		IsSystem:    false,
		Status:      1,
	}

	if template.ID != 1 {
		t.Errorf("Expected ID 1, got %d", template.ID)
	}

	if template.Name != "文案生成模板" {
		t.Errorf("Expected Name '文案生成模板', got %s", template.Name)
	}
	if template.Type != AIGenerationTypeCopywriting {
		t.Errorf("Expected Type 'copywriting', got %s", template.Type)
	}
	if template.Template != "请为{{product}}生成一篇营销文案，目标用户是{{target_audience}}" {
		t.Errorf("Expected Template, got %s", template.Template)
	}
	if template.Variables != `{"product": "产品名称", "target_audience": "目标用户"}` {
		t.Errorf("Expected Variables, got %s", template.Variables)
	}
	if template.Description != "用于生成产品营销文案的模板" {
		t.Errorf("Expected Description, got %s", template.Description)
	}
	if template.Example != "这款产品设计精美，功能强大，非常适合年轻人使用。" {
		t.Errorf("Expected Example, got %s", template.Example)
	}
	if template.IsSystem {
		t.Error("Expected IsSystem to be false")
	}
	if template.Status != 1 {
		t.Errorf("Expected Status 1, got %d", template.Status)
	}
}

func TestPromptTemplate_WithSystemTemplate(t *testing.T) {
	template := &PromptTemplate{
		Name:     "系统模板",
		Type:     AIGenerationTypeReply,
		IsSystem: true,
		Status:   1,
	}

	if !template.IsSystem {
		t.Error("Expected IsSystem to be true")
	}
}

func TestPromptTemplate_WithStatusValues(t *testing.T) {
	statuses := []int{0, 1}

	for _, status := range statuses {
		template := &PromptTemplate{
			Status: status,
		}
		if template.Status != status {
			t.Errorf("Expected Status %d, got %d", status, template.Status)
		}
	}
}
