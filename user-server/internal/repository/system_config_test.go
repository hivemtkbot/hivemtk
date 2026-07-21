package repository

import (
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"testing"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupSystemConfigTestDB 设置系统配置测试数据库
func setupSystemConfigTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.SystemConfig{},
	)
	db.SetTestDB(database)
	return database
}

// setupSystemConfigRepository 创建测试用的系统配置仓库实例
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
				Name:              "app_config",
				WebsiteURL:        "https://example.com",
				AutoReplyHeadless: true,
			},
			wantErr: false,
		},
		{
			name: "create config with minimal fields",
			config: &model.SystemConfig{
				Name:              "minimal_config",
				AutoReplyHeadless: false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.SaveConfig(tt.config)

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

	// 创建测试配置
	expectedConfig := &model.SystemConfig{
		Name:              "test_config",
		WebsiteURL:        "https://test.example.com",
		AutoReplyHeadless: true,
	}
	repo.SaveConfig(expectedConfig)

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
			result, err := repo.GetConfig()

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
				if result.AutoReplyHeadless != true {
					t.Errorf("Expected AutoReplyHeadless true, got %v", result.AutoReplyHeadless)
				}
			}
		})
	}
}

// TestSystemConfigRepository_SaveConfig_Update 测试 SaveConfig 不会更新现有配置
func TestSystemConfigRepository_SaveConfig_Update(t *testing.T) {
	repo := setupSystemConfigRepository(t)

	// 先创建配置
	config := &model.SystemConfig{
		Name:              "update_test",
		WebsiteURL:        "https://original.example.com",
		AutoReplyHeadless: false,
	}
	_, err := repo.SaveConfig(config)
	if err != nil {
		t.Errorf("SaveConfig() create error = %v", err)
	}

	// 注意：SaveConfig 使用 FirstOrCreate，不会更新现有记录
	// 这个测试验证行为：尝试用相同的 Name 创建不会覆盖
	config.WebsiteURL = "https://updated.example.com"
	config.AutoReplyHeadless = true

	// FirstOrCreate 会找到已存在的记录，不会更新
	resultConfig, err := repo.SaveConfig(config)
	if err != nil {
		t.Errorf("SaveConfig() error = %v", err)
	}

	// 验证 FirstOrCreate 保留了原始值而不是更新
	if resultConfig.WebsiteURL != "https://original.example.com" {
		t.Errorf("Expected FirstOrCreate to keep original website URL 'https://original.example.com', got '%s'", resultConfig.WebsiteURL)
	}
}

// TestSystemConfigRepository_GetConfig_Empty 测试获取空配置
func TestSystemConfigRepository_GetConfig_Empty(t *testing.T) {
	setupSystemConfigTestDB(t)

	repo := NewSystemConfigRepository()
	_, err := repo.GetConfig()
	if err == nil {
		t.Error("Expected error for empty config")
	}
}
