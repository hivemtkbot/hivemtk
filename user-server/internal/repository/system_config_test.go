package repository

import (
	"context"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"testing"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func setupSystemConfigTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.SystemConfig{},
	)
	db.SetTestDB(database)
	return database
}

func setupSystemConfigRepository(t *testing.T) SystemConfigRepository {
	setupSystemConfigTestDB(t)
	return NewSystemConfigRepository()
}

// TestSystemConfigRepository_SaveConfig 测试保存系统配置
func TestSystemConfigRepository_SaveConfig(t *testing.T) {
	repo := setupSystemConfigRepository(t)

	tests := []struct {
		name    string
		config  *model.SystemConfig
		wantErr bool
	}{
		{
			name: "create new config",
			config: &model.SystemConfig{
				Name:       "app_config",
				WebsiteURL: "https://example.com",
			},
			wantErr: false,
		},
		{
			name: "create config with minimal fields",
			config: &model.SystemConfig{
				Name: "minimal_config",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.SaveConfig(context.Background(), tt.config)

			if (err != nil) != tt.wantErr {
				t.Errorf("SaveConfig() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.Name != tt.config.Name {
					t.Errorf("Expected name '%s', got '%s'", tt.config.Name, result.Name)
				}
			}
		})
	}
}

// TestSystemConfigRepository_GetConfig 测试获取系统配置
func TestSystemConfigRepository_GetConfig(t *testing.T) {
	repo := setupSystemConfigRepository(t)

	expectedConfig := &model.SystemConfig{
		Name:       "test_config",
		WebsiteURL: "https://test.example.com",
	}
	repo.SaveConfig(context.Background(), expectedConfig)

	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "get existing config",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.GetConfig(context.Background())

			if (err != nil) != tt.wantErr {
				t.Errorf("GetConfig() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.Name != "test_config" {
					t.Errorf("Expected name 'test_config', got '%s'", result.Name)
				}
				if result.WebsiteURL != "https://test.example.com" {
					t.Errorf("Expected website URL 'https://test.example.com', got '%s'", result.WebsiteURL)
				}
			}
		})
	}
}

// TestSystemConfigRepository_SaveConfig_Update 测试 SaveConfig 不会更新现有配置
func TestSystemConfigRepository_SaveConfig_Update(t *testing.T) {
	repo := setupSystemConfigRepository(t)

	config := &model.SystemConfig{
		Name:       "update_test",
		WebsiteURL: "https://original.example.com",
	}
	_, err := repo.SaveConfig(context.Background(), config)
	if err != nil {
		t.Errorf("SaveConfig() create error = %v", err)
	}

	config.WebsiteURL = "https://updated.example.com"

	resultConfig, err := repo.SaveConfig(context.Background(), config)
	if err != nil {
		t.Errorf("SaveConfig() error = %v", err)
	}

	if resultConfig.WebsiteURL != "https://original.example.com" {
		t.Errorf("Expected FirstOrCreate to keep original website URL 'https://original.example.com', got '%s'", resultConfig.WebsiteURL)
	}
}

// TestSystemConfigRepository_GetConfig_Empty 测试获取空配置
func TestSystemConfigRepository_GetConfig_Empty(t *testing.T) {
	setupSystemConfigTestDB(t)

	repo := NewSystemConfigRepository()
	_, err := repo.GetConfig(context.Background())
	if err == nil {
		t.Error("Expected error for empty config")
	}
}
