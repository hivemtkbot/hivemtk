package model

import (
	"testing"
)

func TestMarketTemplate_TableName(t *testing.T) {
	template := &MarketTemplate{}
	tableName := template.TableName()
	if tableName != "market_templates" {
		t.Errorf("Expected table name 'market_templates', got %s", tableName)
	}
}

func TestMarketTemplate_BasicFields(t *testing.T) {
	template := &MarketTemplate{
		ID:            1,
		Name:          "Welcome Flow Template",
		Description:   "A template for welcome marketing flow",
		Category:      "flow",
		Type:          "flow",
		Content:       `{"nodes": []}`,
		Preview:       "https://example.com/preview.png",
		Author:        "Official",
		DownloadCount: 100,
		Rating:        4.5,
		IsOfficial:    true,
		IsFree:        true,
		Price:         0,
	}

	if template.ID != 1 {
		t.Errorf("Expected ID 1, got %d", template.ID)
	}
	if template.Name != "Welcome Flow Template" {
		t.Errorf("Expected Name 'Welcome Flow Template', got %s", template.Name)
	}
	if template.Type != "flow" {
		t.Errorf("Expected Type 'flow', got %s", template.Type)
	}
	if template.DownloadCount != 100 {
		t.Errorf("Expected DownloadCount 100, got %d", template.DownloadCount)
	}
	if template.Rating != 4.5 {
		t.Errorf("Expected Rating 4.5, got %f", template.Rating)
	}
	if !template.IsOfficial {
		t.Error("Expected IsOfficial to be true")
	}
	if !template.IsFree {
		t.Error("Expected IsFree to be true")
	}
}

func TestMarketTemplate_DefaultValues(t *testing.T) {
	template := &MarketTemplate{}

	if template.DownloadCount != 0 {
		t.Logf("DownloadCount is %d (expected 0 before save)", template.DownloadCount)
	}
	if template.Rating != 0 {
		t.Logf("Rating is %f (expected 0 before save)", template.Rating)
	}
	if template.IsOfficial != false {
		t.Logf("IsOfficial is %v (expected false before save)", template.IsOfficial)
	}
	if template.IsFree != false {
		t.Logf("IsFree is %v (expected false before save)", template.IsFree)
	}
	if template.Price != 0 {
		t.Logf("Price is %d (expected 0 before save)", template.Price)
	}
}

func TestMarketTemplate_TypeValues(t *testing.T) {
	types := []string{"flow", "report", "script", "email"}

	for _, tmplType := range types {
		template := &MarketTemplate{
			Type: tmplType,
		}
		if template.Type != tmplType {
			t.Errorf("Expected Type %s, got %s", tmplType, template.Type)
		}
	}
}

func TestMarketTemplateDownload_TableName(t *testing.T) {
	download := &MarketTemplateDownload{}
	tableName := download.TableName()
	if tableName != "market_template_downloads" {
		t.Errorf("Expected table name 'market_template_downloads', got %s", tableName)
	}
}

func TestMarketTemplateDownload_BasicFields(t *testing.T) {
	download := &MarketTemplateDownload{
		ID:           1,
		TemplateID:   100,
		TemplateType: "flow",
	}

	if download.ID != 1 {
		t.Errorf("Expected ID 1, got %d", download.ID)
	}

	if download.TemplateID != 100 {
		t.Errorf("Expected TemplateID 100, got %d", download.TemplateID)
	}
	if download.TemplateType != "flow" {
		t.Errorf("Expected TemplateType 'flow', got %s", download.TemplateType)
	}
}
