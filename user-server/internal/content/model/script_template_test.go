package model

import (
	"testing"
	"time"
)

func TestScriptTemplate_TableName(t *testing.T) {
	template := &ScriptTemplate{}
	tableName := template.TableName()
	if tableName != "script_templates" {
		t.Errorf("Expected table name 'script_templates', got %s", tableName)
	}
}

func TestScriptTemplate_BasicFields(t *testing.T) {
	template := &ScriptTemplate{
		ID:         1,
		CategoryID: 10,
		Category:   "greeting",
		Title:      "Welcome Message",
		Content:    "Hello! Welcome to our store. How can I help you today?",
		Variables:  `["customer_name", "product_name"]`,
		Tags:       "greeting,welcome",
		UsageCount: 100,
		Rating:     4.5,
		IsPublic:   true,
		IsSystem:   false,
		CreatedBy:  100,
	}

	if template.ID != 1 {
		t.Errorf("Expected ID 1, got %d", template.ID)
	}
	if template.Title != "Welcome Message" {
		t.Errorf("Expected Title 'Welcome Message', got %s", template.Title)
	}
	if template.Category != "greeting" {
		t.Errorf("Expected Category 'greeting', got %s", template.Category)
	}
	if template.UsageCount != 100 {
		t.Errorf("Expected UsageCount 100, got %d", template.UsageCount)
	}
	if template.Rating != 4.5 {
		t.Errorf("Expected Rating 4.5, got %f", template.Rating)
	}
	if !template.IsPublic {
		t.Error("Expected IsPublic to be true")
	}
	if template.IsSystem {
		t.Error("Expected IsSystem to be false")
	}
}

func TestScriptTemplate_DefaultValues(t *testing.T) {
	template := &ScriptTemplate{}

	if template.UsageCount != 0 {
		t.Logf("UsageCount is %d (expected 0 before save)", template.UsageCount)
	}
	if template.Rating != 0 {
		t.Logf("Rating is %f (expected 0 before save)", template.Rating)
	}
	if template.IsPublic != false {
		t.Logf("IsPublic is %v (expected false before save)", template.IsPublic)
	}
	if template.IsSystem != false {
		t.Logf("IsSystem is %v (expected false before save)", template.IsSystem)
	}
}

func TestScriptCategory_TableName(t *testing.T) {
	category := &ScriptCategory{}
	tableName := category.TableName()
	if tableName != "script_categories" {
		t.Errorf("Expected table name 'script_categories', got %s", tableName)
	}
}

func TestScriptCategory_BasicFields(t *testing.T) {
	category := &ScriptCategory{
		ID:        1,
		Name:      "Greetings",
		ParentID:  0,
		SortOrder: 1,
	}

	if category.ID != 1 {
		t.Errorf("Expected ID 1, got %d", category.ID)
	}
	if category.Name != "Greetings" {
		t.Errorf("Expected Name 'Greetings', got %s", category.Name)
	}
	if category.ParentID != 0 {
		t.Errorf("Expected ParentID 0, got %d", category.ParentID)
	}
	if category.SortOrder != 1 {
		t.Errorf("Expected SortOrder 1, got %d", category.SortOrder)
	}
}

func TestScriptRecommend_TableName(t *testing.T) {
	recommend := &ScriptRecommend{}
	tableName := recommend.TableName()
	if tableName != "script_recommendations" {
		t.Errorf("Expected table name 'script_recommendations', got %s", tableName)
	}
}

func TestScriptRecommend_BasicFields(t *testing.T) {
	now := time.Now()

	recommend := &ScriptRecommend{
		ID:            1,
		SessionID:     "sess-001",
		Message:       "I want to know about shipping",
		TemplateID:    10,
		TemplateTitle: "Shipping Info",
		Confidence:    0.85,
		IsUsed:        true,
		UsedAt:        &now,
	}

	if recommend.ID != 1 {
		t.Errorf("Expected ID 1, got %d", recommend.ID)
	}
	if recommend.TemplateTitle != "Shipping Info" {
		t.Errorf("Expected TemplateTitle 'Shipping Info', got %s", recommend.TemplateTitle)
	}
	if recommend.Confidence != 0.85 {
		t.Errorf("Expected Confidence 0.85, got %f", recommend.Confidence)
	}
	if !recommend.IsUsed {
		t.Error("Expected IsUsed to be true")
	}
}

func TestScriptRecommend_DefaultValues(t *testing.T) {
	recommend := &ScriptRecommend{}

	if recommend.IsUsed != false {
		t.Logf("IsUsed is %v (expected false before use)", recommend.IsUsed)
	}
	if recommend.Confidence != 0 {
		t.Logf("Confidence is %f (expected 0 before save)", recommend.Confidence)
	}
}

