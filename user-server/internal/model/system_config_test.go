package model

import (
	"testing"
)

func TestSystemConfig_TableName(t *testing.T) {
	config := &SystemConfig{}
	tableName := config.TableName()
	if tableName != "system_config" {
		t.Errorf("Expected table name 'system_config', got %s", tableName)
	}
}

func TestSystemConfig_BasicFields(t *testing.T) {
	config := &SystemConfig{
		Name:       "Test Config",
		WebsiteURL: "https://example.com",
	}

	if config.Name != "Test Config" {
		t.Errorf("Expected Name 'Test Config', got %s", config.Name)
	}
	if config.WebsiteURL != "https://example.com" {
		t.Errorf("Expected WebsiteURL 'https://example.com', got %s", config.WebsiteURL)
	}
}

func TestSystemConfig_DefaultValues(t *testing.T) {
	config := &SystemConfig{}

	if config.Name != "" {
		t.Errorf("Expected empty Name, got %s", config.Name)
	}
	if config.WebsiteURL != "" {
		t.Errorf("Expected empty WebsiteURL, got %s", config.WebsiteURL)
	}
}

func TestSystemConfig_WithEmptyName(t *testing.T) {
	config := &SystemConfig{
		Name: "",
	}

	if config.Name != "" {
		t.Errorf("Expected empty Name, got %s", config.Name)
	}
}
