package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/repository"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupSystemConfigServiceTestDB 设置系统配置服务测试数据库
func setupSystemConfigServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.SystemConfig{},
	)
	db.SetTestDB(database)
	return database
}

// newTestSystemConfigRepository 创建测试仓库
func newTestSystemConfigRepository(database *gorm.DB) repository.SystemConfigRepository {
	return repository.NewSystemConfigRepository()
}

// TestNewSystemConfigService 测试创建系统配置服务
func TestNewSystemConfigService(t *testing.T) {
	service := NewSystemConfigService()
	if service == nil {
		t.Error("Expected non-nil service")
	}
}

// TestSystemConfigService_GetConfig_Empty 测试获取空配置
func TestSystemConfigService_GetConfig_Empty(t *testing.T) {
	database := setupSystemConfigServiceTestDB(t)
	repo := newTestSystemConfigRepository(database)
	service := &SystemConfigService{repo: repo}

	config, err := service.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}

	if config == nil {
		t.Fatal("Expected non-nil config")
	}

	if config.Name != "" {
		t.Errorf("Expected empty name, got %s", config.Name)
	}

	if config.WebsiteURL != "" {
		t.Errorf("Expected empty website URL, got %s", config.WebsiteURL)
	}
}

// TestSystemConfigService_GetConfig_WithDB 测试从数据库获取配置
func TestSystemConfigService_GetConfig_WithDB(t *testing.T) {
	database := setupSystemConfigServiceTestDB(t)
	repo := newTestSystemConfigRepository(database)
	service := &SystemConfigService{repo: repo}

	config := &model.SystemConfig{
		Name:       "测试系统",
		WebsiteURL: "https://example.com",
	}
	database.Create(config)

	retrievedConfig, err := service.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}

	if retrievedConfig.Name != "测试系统" {
		t.Errorf("Expected name '测试系统', got %s", retrievedConfig.Name)
	}

	if retrievedConfig.WebsiteURL != "https://example.com" {
		t.Errorf("Expected website URL 'https://example.com', got %s", retrievedConfig.WebsiteURL)
	}
}

// TestSystemConfigService_SaveConfig 测试保存配置
func TestSystemConfigService_SaveConfig(t *testing.T) {
	database := setupSystemConfigServiceTestDB(t)
	repo := newTestSystemConfigRepository(database)
	service := &SystemConfigService{repo: repo}

	newConfig := &model.SystemConfig{
		Name:       "新系统",
		WebsiteURL: "https://newexample.com",
	}

	savedConfig, err := service.SaveConfig(context.Background(), newConfig)
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	if savedConfig.Name != "新系统" {
		t.Errorf("Expected name '新系统', got %s", savedConfig.Name)
	}

	if savedConfig.WebsiteURL != "https://newexample.com" {
		t.Errorf("Expected website URL 'https://newexample.com', got %s", savedConfig.WebsiteURL)
	}

	// 验证配置已保存到数据库
	var count int64
	database.Model(&model.SystemConfig{}).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 config record, got %d", count)
	}
}

// TestSystemConfigService_SaveConfig_EmptyFields 测试保存空字段配置
func TestSystemConfigService_SaveConfig_EmptyFields(t *testing.T) {
	database := setupSystemConfigServiceTestDB(t)
	repo := newTestSystemConfigRepository(database)
	service := &SystemConfigService{repo: repo}

	emptyConfig := &model.SystemConfig{
		Name:       "",
		WebsiteURL: "",
	}

	savedConfig, err := service.SaveConfig(context.Background(), emptyConfig)
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	if savedConfig.Name != "" {
		t.Errorf("Expected empty name, got %s", savedConfig.Name)
	}

	if savedConfig.WebsiteURL != "" {
		t.Errorf("Expected empty website URL, got %s", savedConfig.WebsiteURL)
	}
}

// TestSystemConfigService_SaveConfig_WithSpecialChars 测试保存含特殊字符的配置
func TestSystemConfigService_SaveConfig_WithSpecialChars(t *testing.T) {
	database := setupSystemConfigServiceTestDB(t)
	repo := newTestSystemConfigRepository(database)
	service := &SystemConfigService{repo: repo}

	specialConfig := &model.SystemConfig{
		Name:       "系统 <测试> & \"引用\"",
		WebsiteURL: "https://example.com/path?query=value&param=test",
	}

	savedConfig, err := service.SaveConfig(context.Background(), specialConfig)
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	if savedConfig.Name != "系统 <测试> & \"引用\"" {
		t.Errorf("Expected name with special chars, got %s", savedConfig.Name)
	}

	if savedConfig.WebsiteURL != "https://example.com/path?query=value&param=test" {
		t.Errorf("Expected URL with special chars, got %s", savedConfig.WebsiteURL)
	}
}

// TestSystemConfigService_SaveConfig_LongFields 测试保存长字段配置
func TestSystemConfigService_SaveConfig_LongFields(t *testing.T) {
	database := setupSystemConfigServiceTestDB(t)
	repo := newTestSystemConfigRepository(database)
	service := &SystemConfigService{repo: repo}

	longName := ""
	for i := 0; i < 200; i++ {
		longName += "a"
	}

	longURL := "https://"
	for i := 0; i < 200; i++ {
		longURL += "x"
	}
	longURL += ".com"

	longConfig := &model.SystemConfig{
		Name:       longName,
		WebsiteURL: longURL,
	}

	savedConfig, err := service.SaveConfig(context.Background(), longConfig)
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	if len(savedConfig.Name) != 200 {
		t.Errorf("Expected name length 200, got %d", len(savedConfig.Name))
	}
}

// TestSystemConfigService_SaveConfig_MultipleUpdates 测试多次更新配置
func TestSystemConfigService_SaveConfig_MultipleUpdates(t *testing.T) {
	database := setupSystemConfigServiceTestDB(t)
	repo := newTestSystemConfigRepository(database)
	service := &SystemConfigService{repo: repo}

	config1 := &model.SystemConfig{
		Name:       "配置 1",
		WebsiteURL: "https://example1.com",
	}
	service.SaveConfig(context.Background(), config1)

	config2 := &model.SystemConfig{
		Name:       "配置 2",
		WebsiteURL: "https://example2.com",
	}
	service.SaveConfig(context.Background(), config2)

	// 验证最终配置
	var count int64
	database.Model(&model.SystemConfig{}).Count(&count)
	if count != 1 {
		t.Errorf("Expected only 1 config record, got %d", count)
	}

	finalConfig, err := service.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}

	if finalConfig.Name == "" && finalConfig.WebsiteURL == "" {
		t.Error("Expected non-empty config after multiple saves")
	}
}

// TestSystemConfigService_Integration 测试集成场景
func TestSystemConfigService_Integration(t *testing.T) {
	database := setupSystemConfigServiceTestDB(t)
	repo := newTestSystemConfigRepository(database)
	service := &SystemConfigService{repo: repo}

	config, err := service.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}
	if config.Name != "" {
		t.Errorf("Expected empty initial name, got %s", config.Name)
	}

	newConfig := &model.SystemConfig{
		Name:       "集成测试系统",
		WebsiteURL: "https://test.example.com",
	}
	savedConfig, err := service.SaveConfig(context.Background(), newConfig)
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	if savedConfig.Name != "集成测试系统" {
		t.Errorf("Expected saved name '集成测试系统', got %s", savedConfig.Name)
	}

	finalConfig, err := service.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}

	if finalConfig.Name != "集成测试系统" {
		t.Errorf("Expected final name '集成测试系统', got %s", finalConfig.Name)
	}
}
