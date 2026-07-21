package service

import (
	"mime/multipart"
	"strings"
	"testing"
	"time"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/repository"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupObsConfigServiceTestDB 设置 OBS 配置服务测试数据库
func setupObsConfigServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.ObsConfig{},
		&model.License{},
	)
	db.SetTestDB(database)
	return database
}

func newTestObsConfigService(database *gorm.DB) ObsConfigService {
	repo := repository.NewObsConfigRepositoryWithDB(database)
	return &obsConfigService{repo: repo}
}

func TestNewObsConfigService(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)
	if service == nil {
		t.Error("Expected non-nil service")
	}
}

func TestObsConfigService_GetConfigList(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	license := &model.License{
		Key:          "TEST12345678901234567890123456789012",
		MerchantName: "测试商户",
		ExpireAt:     time.Now().Add(365 * 24 * time.Hour),
	}
	database.Create(license)

	for i := 0; i < 5; i++ {
		database.Create(&model.ObsConfig{
			Name:      "配置" + string(rune('0'+i)),
			Provider:  model.ObsProviderAliyun,
			AccessKey: "test-key",
			SecretKey: "test-secret",
			Bucket:    "test-bucket",
			LicenseID: license.ID,
		})
	}

	response, err := service.GetConfigList(license.ID, 1, 10)
	if err != nil {
		t.Fatalf("GetConfigList failed: %v", err)
	}

	if response.Total != 5 {
		t.Errorf("Expected total 5, got %d", response.Total)
	}
	if len(response.List) != 5 {
		t.Errorf("Expected 5 configs, got %d", len(response.List))
	}
}

func TestObsConfigService_GetConfigList_WithPagination(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	license := &model.License{
		Key:          "TEST12345678901234567890123456789012",
		MerchantName: "测试商户",
		ExpireAt:     time.Now().Add(365 * 24 * time.Hour),
	}
	database.Create(license)

	for i := 0; i < 15; i++ {
		database.Create(&model.ObsConfig{
			Name:      "配置" + string(rune('0'+i%10)),
			Provider:  model.ObsProviderAliyun,
			AccessKey: "test-key",
			SecretKey: "test-secret",
			Bucket:    "test-bucket",
			LicenseID: license.ID,
		})
	}

	response, err := service.GetConfigList(license.ID, 1, 5)
	if err != nil {
		t.Fatalf("GetConfigList failed: %v", err)
	}

	if response.Total != 15 {
		t.Errorf("Expected total 15, got %d", response.Total)
	}
	if len(response.List) != 5 {
		t.Errorf("Expected 5 configs on page 1, got %d", len(response.List))
	}

	response2, err := service.GetConfigList(license.ID, 2, 5)
	if err != nil {
		t.Fatalf("GetConfigList failed: %v", err)
	}
	if len(response2.List) != 5 {
		t.Errorf("Expected 5 configs on page 2, got %d", len(response2.List))
	}
}

func TestObsConfigService_GetConfig(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	config := &model.ObsConfig{
		Name:      "测试配置",
		Provider:  model.ObsProviderAliyun,
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
		Region:    "cn-hangzhou",
		IsDefault: true,
		MaxSize:   104857600,
		MaxCount:  1000,
	}
	database.Create(config)

	response, err := service.GetConfig(config.ID)
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}

	if response.Name != "测试配置" {
		t.Errorf("Expected name '测试配置', got %s", response.Name)
	}
	if response.Provider != "aliyun" {
		t.Errorf("Expected provider 'aliyun', got %s", response.Provider)
	}
	if response.ProviderName != "阿里云OSS" {
		t.Errorf("Expected providerName '阿里云OSS', got %s", response.ProviderName)
	}
	if response.SecretKey != "***" {
		t.Errorf("Expected secretKey '***', got %s", response.SecretKey)
	}
}

func TestObsConfigService_GetConfig_NotFound(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	_, err := service.GetConfig("non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent config")
	}
}

func TestObsConfigService_CreateConfig(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	license := &model.License{
		Key:          "TEST12345678901234567890123456789012",
		MerchantName: "测试商户",
		ExpireAt:     time.Now().Add(365 * 24 * time.Hour),
	}
	database.Create(license)

	req := &dto.CreateObsConfigRequest{
		Name:      "测试配置",
		Provider:  "aliyun",
		AccessKey: "test-access-key",
		SecretKey: "test-secret-key",
		Bucket:    "test-bucket",
		Region:    "cn-hangzhou",
		MaxSize:   104857600,
		MaxCount:  1000,
		LicenseID: license.ID,
	}

	response, err := service.CreateConfig(req)
	if err != nil {
		t.Fatalf("CreateConfig failed: %v", err)
	}

	if response.Name != "测试配置" {
		t.Errorf("Expected name '测试配置', got %s", response.Name)
	}

	var count int64
	database.Model(&model.ObsConfig{}).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 config, got %d", count)
	}
}

func TestObsConfigService_CreateConfig_FirstConfigIsDefault(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	license := &model.License{
		Key:          "TEST12345678901234567890123456789012",
		MerchantName: "测试商户",
		ExpireAt:     time.Now().Add(365 * 24 * time.Hour),
	}
	database.Create(license)

	req := &dto.CreateObsConfigRequest{
		Name:      "第一个配置",
		Provider:  "aliyun",
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
		LicenseID: license.ID,
	}

	response, err := service.CreateConfig(req)
	if err != nil {
		t.Fatalf("CreateConfig failed: %v", err)
	}

	if !response.IsDefault {
		t.Error("Expected first config to be default")
	}

	req2 := &dto.CreateObsConfigRequest{
		Name:      "第二个配置",
		Provider:  "tencent",
		AccessKey: "test-key-2",
		SecretKey: "test-secret-2",
		Bucket:    "test-bucket-2",
		LicenseID: license.ID,
	}

	response2, err := service.CreateConfig(req2)
	if err != nil {
		t.Fatalf("CreateConfig failed: %v", err)
	}

	if response2.IsDefault {
		t.Error("Expected second config not to be default")
	}
}

func TestObsConfigService_CreateConfig_EmptyName(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	req := &dto.CreateObsConfigRequest{
		Name:      "",
		Provider:  "aliyun",
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
		LicenseID: "test-license",
	}

	_, err := service.CreateConfig(req)
	if err == nil {
		t.Error("Expected error for empty name")
	}
}

func TestObsConfigService_CreateConfig_EmptyProvider(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	req := &dto.CreateObsConfigRequest{
		Name:      "测试配置",
		Provider:  "",
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
		LicenseID: "test-license",
	}

	_, err := service.CreateConfig(req)
	if err == nil {
		t.Error("Expected error for empty provider")
	}
}

func TestObsConfigService_CreateConfig_EmptyAccessKey(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	req := &dto.CreateObsConfigRequest{
		Name:      "测试配置",
		Provider:  "aliyun",
		AccessKey: "",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
		LicenseID: "test-license",
	}

	_, err := service.CreateConfig(req)
	if err == nil {
		t.Error("Expected error for empty access key")
	}
}

func TestObsConfigService_CreateConfig_EmptySecretKey(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	req := &dto.CreateObsConfigRequest{
		Name:      "测试配置",
		Provider:  "aliyun",
		AccessKey: "test-key",
		SecretKey: "",
		Bucket:    "test-bucket",
		LicenseID: "test-license",
	}

	_, err := service.CreateConfig(req)
	if err == nil {
		t.Error("Expected error for empty secret key")
	}
}

func TestObsConfigService_CreateConfig_EmptyBucket(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	req := &dto.CreateObsConfigRequest{
		Name:      "测试配置",
		Provider:  "aliyun",
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "",
		LicenseID: "test-license",
	}

	_, err := service.CreateConfig(req)
	if err == nil {
		t.Error("Expected error for empty bucket")
	}
}

func TestObsConfigService_UpdateConfig(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	config := &model.ObsConfig{
		Name:      "旧名称",
		Provider:  model.ObsProviderAliyun,
		AccessKey: "old-key",
		SecretKey: "old-secret",
		Bucket:    "old-bucket",
		MaxSize:   104857600,
		MaxCount:  1000,
	}
	database.Create(config)

	req := &dto.UpdateObsConfigRequest{
		Name:     "新名称",
		Bucket:   "new-bucket",
		MaxSize:  209715200,
		MaxCount: 2000,
	}

	response, err := service.UpdateConfig(config.ID, req)
	if err != nil {
		t.Fatalf("UpdateConfig failed: %v", err)
	}

	if response.Name != "新名称" {
		t.Errorf("Expected name '新名称', got %s", response.Name)
	}
	if response.Bucket != "new-bucket" {
		t.Errorf("Expected bucket 'new-bucket', got %s", response.Bucket)
	}
	if response.MaxSize != 209715200 {
		t.Errorf("Expected max size 209715200, got %d", response.MaxSize)
	}
}

func TestObsConfigService_UpdateConfig_NotFound(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	req := &dto.UpdateObsConfigRequest{Name: "新名称"}
	_, err := service.UpdateConfig("non-existent-id", req)
	if err == nil {
		t.Error("Expected error for non-existent config")
	}
}

func TestObsConfigService_UpdateConfig_PartialUpdate(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	config := &model.ObsConfig{
		Name:      "测试配置",
		Provider:  model.ObsProviderAliyun,
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
		Region:    "cn-hangzhou",
		MaxSize:   104857600,
		MaxCount:  1000,
	}
	database.Create(config)

	req := &dto.UpdateObsConfigRequest{Name: "新名称"}

	response, err := service.UpdateConfig(config.ID, req)
	if err != nil {
		t.Fatalf("UpdateConfig failed: %v", err)
	}

	if response.Name != "新名称" {
		t.Errorf("Expected name '新名称', got %s", response.Name)
	}
	if response.Region != "cn-hangzhou" {
		t.Errorf("Expected region 'cn-hangzhou', got %s", response.Region)
	}
}

func TestObsConfigService_DeleteConfig(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	config := &model.ObsConfig{
		Name:      "待删除配置",
		Provider:  model.ObsProviderAliyun,
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
		IsDefault: false,
	}
	database.Create(config)

	err := service.DeleteConfig(config.ID)
	if err != nil {
		t.Fatalf("DeleteConfig failed: %v", err)
	}

	var count int64
	database.Model(&model.ObsConfig{}).Where("id = ?", config.ID).Count(&count)
	if count != 0 {
		t.Errorf("Expected config to be deleted, got count %d", count)
	}
}

func TestObsConfigService_DeleteConfig_Default(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	config := &model.ObsConfig{
		Name:      "默认配置",
		Provider:  model.ObsProviderAliyun,
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
		IsDefault: true,
	}
	database.Create(config)

	err := service.DeleteConfig(config.ID)
	if err == nil {
		t.Error("Expected error for deleting default config")
	}
	if !strings.Contains(err.Error(), "不能删除默认配置") {
		t.Errorf("Expected '不能删除默认配置', got %s", err.Error())
	}
}

func TestObsConfigService_DeleteConfig_NotFound(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	err := service.DeleteConfig("non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent config")
	}
}

func TestObsConfigService_SetDefaultConfig(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	license := &model.License{
		Key:          "TEST12345678901234567890123456789012",
		MerchantName: "测试商户",
		ExpireAt:     time.Now().Add(365 * 24 * time.Hour),
	}
	database.Create(license)

	config1 := &model.ObsConfig{
		Name:      "配置 1",
		Provider:  model.ObsProviderAliyun,
		AccessKey: "test-key-1",
		SecretKey: "test-secret-1",
		Bucket:    "test-bucket-1",
		LicenseID: license.ID,
		IsDefault: true,
	}
	database.Create(config1)

	config2 := &model.ObsConfig{
		Name:      "配置 2",
		Provider:  model.ObsProviderTencent,
		AccessKey: "test-key-2",
		SecretKey: "test-secret-2",
		Bucket:    "test-bucket-2",
		LicenseID: license.ID,
		IsDefault: false,
	}
	database.Create(config2)

	err := service.SetDefaultConfig(config2.ID, license.ID)
	if err != nil {
		t.Fatalf("SetDefaultConfig failed: %v", err)
	}

	var updatedConfig1, updatedConfig2 model.ObsConfig
	database.Where("id = ?", config1.ID).First(&updatedConfig1)
	database.Where("id = ?", config2.ID).First(&updatedConfig2)

	if updatedConfig1.IsDefault {
		t.Error("Expected config1 not to be default")
	}
	if !updatedConfig2.IsDefault {
		t.Error("Expected config2 to be default")
	}
}

func TestObsConfigService_SetDefaultConfig_WrongLicense(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	config := &model.ObsConfig{
		Name:      "测试配置",
		Provider:  model.ObsProviderAliyun,
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
		LicenseID: "license-1",
	}
	database.Create(config)

	err := service.SetDefaultConfig(config.ID, "license-2")
	if err == nil {
		t.Error("Expected error for wrong license")
	}
	if !strings.Contains(err.Error(), "配置不属于该许可证") {
		t.Errorf("Expected '配置不属于该许可证', got %s", err.Error())
	}
}

func TestObsConfigService_SetDefaultConfig_NotFound(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	err := service.SetDefaultConfig("non-existent-id", "license-1")
	if err == nil {
		t.Error("Expected error for non-existent config")
	}
}

func TestObsConfigService_TestConnection_Aliyun(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	config := &dto.ObsConfigResponse{
		Provider:  "aliyun",
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
		Region:    "cn-hangzhou",
	}

	err := service.TestConnection(config)
	if err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}
}

func TestObsConfigService_TestConnection_Qiniu(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	config := &dto.ObsConfigResponse{
		Provider:  "qiniu",
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
	}

	err := service.TestConnection(config)
	if err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}
}

func TestObsConfigService_TestConnection_Tencent(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	config := &dto.ObsConfigResponse{
		Provider:  "tencent",
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
		Region:    "ap-guangzhou",
	}

	err := service.TestConnection(config)
	if err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}
}

func TestObsConfigService_TestConnection_AWS(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	config := &dto.ObsConfigResponse{
		Provider:  "aws",
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
		Region:    "us-east-1",
	}

	err := service.TestConnection(config)
	if err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}
}

func TestObsConfigService_TestConnection_Local(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	config := &dto.ObsConfigResponse{
		Provider:  "local",
		AccessKey: "",
		SecretKey: "",
		Bucket:    "",
	}

	err := service.TestConnection(config)
	if err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}
}

func TestObsConfigService_TestConnection_InvalidProvider(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	config := &dto.ObsConfigResponse{
		Provider:  "unknown",
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
	}

	err := service.TestConnection(config)
	if err == nil {
		t.Error("Expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "不支持的存储提供商") {
		t.Errorf("Expected '不支持的存储提供商', got %s", err.Error())
	}
}

func TestObsConfigService_TestConnection_Aliyun_EmptyKey(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	config := &dto.ObsConfigResponse{
		Provider:  "aliyun",
		AccessKey: "",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
	}

	err := service.TestConnection(config)
	if err == nil {
		t.Error("Expected error for empty access key")
	}
	if !strings.Contains(err.Error(), "阿里云OSS配置不完整") {
		t.Errorf("Expected '阿里云OSS配置不完整', got %s", err.Error())
	}
}

func TestObsConfigService_TestConnection_Aliyun_EmptySecret(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	config := &dto.ObsConfigResponse{
		Provider:  "aliyun",
		AccessKey: "test-key",
		SecretKey: "",
		Bucket:    "test-bucket",
	}

	err := service.TestConnection(config)
	if err == nil {
		t.Error("Expected error for empty secret key")
	}
	if !strings.Contains(err.Error(), "阿里云OSS配置不完整") {
		t.Errorf("Expected '阿里云OSS配置不完整', got %s", err.Error())
	}
}

func TestObsConfigService_TestConnection_Aliyun_EmptyBucket(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	config := &dto.ObsConfigResponse{
		Provider:  "aliyun",
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "",
	}

	err := service.TestConnection(config)
	if err == nil {
		t.Error("Expected error for empty bucket")
	}
	if !strings.Contains(err.Error(), "阿里云OSS配置不完整") {
		t.Errorf("Expected '阿里云OSS配置不完整', got %s", err.Error())
	}
}

func TestObsConfigService_UploadFile(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	license := &model.License{
		Key:          "TEST12345678901234567890123456789012",
		MerchantName: "测试商户",
		ExpireAt:     time.Now().Add(365 * 24 * time.Hour),
	}
	database.Create(license)

	config := &model.ObsConfig{
		Name:      "默认配置",
		Provider:  model.ObsProviderLocal,
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
		LicenseID: license.ID,
		IsDefault: true,
		MaxSize:   104857600,
		MaxCount:  1000,
		FileCount: 0,
		TotalSize: 0,
	}
	database.Create(config)

	fileContent := []byte("test file content")
	file := &mockFile{content: fileContent}
	header := &multipart.FileHeader{
		Filename: "test.txt",
		Size:     int64(len(fileContent)),
	}

	url, err := service.UploadFile(file, header, license.ID, "test-folder")
	if err != nil {
		t.Fatalf("UploadFile failed: %v", err)
	}
	if url == "" {
		t.Error("Expected non-empty file URL")
	}
}

func TestObsConfigService_UploadFile_NoConfig(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	fileContent := []byte("test file content")
	file := &mockFile{content: fileContent}
	header := &multipart.FileHeader{
		Filename: "test.txt",
		Size:     int64(len(fileContent)),
	}

	_, err := service.UploadFile(file, header, "non-existent-license", "test-folder")
	if err == nil {
		t.Error("Expected error for no config")
	}
	if !strings.Contains(err.Error(), "未找到默认存储配置") {
		t.Errorf("Expected '未找到默认存储配置', got %s", err.Error())
	}
}

func TestObsConfigService_UploadFile_FileTooLarge(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	license := &model.License{
		Key:          "TEST12345678901234567890123456789012",
		MerchantName: "测试商户",
		ExpireAt:     time.Now().Add(365 * 24 * time.Hour),
	}
	database.Create(license)

	config := &model.ObsConfig{
		Name:      "默认配置",
		Provider:  model.ObsProviderLocal,
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
		LicenseID: license.ID,
		IsDefault: true,
		MaxSize:   100,
		MaxCount:  1000,
	}
	database.Create(config)

	fileContent := []byte("this is a test file content that exceeds the 100 bytes limit by far, this is a longer string to make sure we go over 100 bytes")
	file := &mockFile{content: fileContent}
	header := &multipart.FileHeader{
		Filename: "test.txt",
		Size:     int64(len(fileContent)),
	}

	_, err := service.UploadFile(file, header, license.ID, "test-folder")
	if err == nil {
		t.Error("Expected error for file too large")
	}
	if err != nil && !strings.Contains(err.Error(), "文件大小超过限制") {
		t.Errorf("Expected file size error, got %s", err.Error())
	}
}

func TestObsConfigService_UploadFile_FileCountLimit(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	license := &model.License{
		Key:          "TEST12345678901234567890123456789012",
		MerchantName: "测试商户",
		ExpireAt:     time.Now().Add(365 * 24 * time.Hour),
	}
	database.Create(license)

	config := &model.ObsConfig{
		Name:      "默认配置",
		Provider:  model.ObsProviderLocal,
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
		LicenseID: license.ID,
		IsDefault: true,
		MaxSize:   104857600,
		MaxCount:  5,
		FileCount: 5,
	}
	database.Create(config)

	fileContent := []byte("test file content")
	file := &mockFile{content: fileContent}
	header := &multipart.FileHeader{
		Filename: "test.txt",
		Size:     int64(len(fileContent)),
	}

	_, err := service.UploadFile(file, header, license.ID, "test-folder")
	if err == nil {
		t.Error("Expected error for file count limit")
	}
	if err != nil && !strings.Contains(err.Error(), "已达到最大文件数量限制") {
		t.Errorf("Expected file count limit error, got %s", err.Error())
	}
}

func TestObsConfigService_GetDefaultConfig(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	license := &model.License{
		Key:          "TEST12345678901234567890123456789012",
		MerchantName: "测试商户",
		ExpireAt:     time.Now().Add(365 * 24 * time.Hour),
	}
	database.Create(license)

	config := &model.ObsConfig{
		Name:      "默认配置",
		Provider:  model.ObsProviderAliyun,
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
		LicenseID: license.ID,
		IsDefault: true,
	}
	database.Create(config)

	response, err := service.GetDefaultConfig(license.ID)
	if err != nil {
		t.Fatalf("GetDefaultConfig failed: %v", err)
	}

	if response.Name != "默认配置" {
		t.Errorf("Expected name '默认配置', got %s", response.Name)
	}
	if !response.IsDefault {
		t.Error("Expected config to be default")
	}
}

func TestObsConfigService_GetDefaultConfig_NotFound(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	_, err := service.GetDefaultConfig("non-existent-license")
	if err == nil {
		t.Error("Expected error for non-existent config")
	}
}

func TestObsConfigService_UploadFile_CloudStorage(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	license := &model.License{
		Key:          "TEST12345678901234567890123456789012",
		MerchantName: "测试商户",
		ExpireAt:     time.Now().Add(365 * 24 * time.Hour),
	}
	database.Create(license)

	config := &model.ObsConfig{
		Name:      "阿里云配置",
		Provider:  model.ObsProviderAliyun,
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
		Region:    "cn-hangzhou",
		LicenseID: license.ID,
		IsDefault: true,
		MaxSize:   104857600,
		MaxCount:  1000,
		FileCount: 0,
		TotalSize: 0,
	}
	database.Create(config)

	fileContent := []byte("test file content")
	file := &mockFile{content: fileContent}
	header := &multipart.FileHeader{
		Filename: "test.txt",
		Size:     int64(len(fileContent)),
	}

	url, err := service.UploadFile(file, header, license.ID, "test-folder")
	if err != nil {
		t.Fatalf("UploadFile failed: %v", err)
	}
	if url == "" {
		t.Error("Expected non-empty file URL")
	}

	var updatedConfig model.ObsConfig
	database.Where("id = ?", config.ID).First(&updatedConfig)
	if updatedConfig.FileCount != 1 {
		t.Errorf("Expected file count 1, got %d", updatedConfig.FileCount)
	}
	if updatedConfig.TotalSize != int64(len(fileContent)) {
		t.Errorf("Expected total size %d, got %d", len(fileContent), updatedConfig.TotalSize)
	}
}

func TestObsConfigService_GetConfigList_EmptyList(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	license := &model.License{
		Key:          "TEST12345678901234567890123456789012",
		MerchantName: "测试商户",
		ExpireAt:     time.Now().Add(365 * 24 * time.Hour),
	}
	database.Create(license)

	response, err := service.GetConfigList(license.ID, 1, 10)
	if err != nil {
		t.Fatalf("GetConfigList failed: %v", err)
	}

	if response.Total != 0 {
		t.Errorf("Expected total 0, got %d", response.Total)
	}
	if len(response.List) != 0 {
		t.Errorf("Expected empty list, got %d", len(response.List))
	}
}

func TestObsConfigService_CreateConfig_WithAllFields(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	license := &model.License{
		Key:          "TEST12345678901234567890123456789012",
		MerchantName: "测试商户",
		ExpireAt:     time.Now().Add(365 * 24 * time.Hour),
	}
	database.Create(license)

	req := &dto.CreateObsConfigRequest{
		Name:       "完整配置",
		Provider:   "aliyun",
		AccessKey:  "test-key",
		SecretKey:  "test-secret",
		Bucket:     "test-bucket",
		Region:     "cn-hangzhou",
		Endpoint:   "https://oss-cn-hangzhou.aliyuncs.com",
		Domain:     "https://cdn.example.com",
		PathPrefix: "/uploads",
		Config:     `{"key": "value"}`,
		MaxSize:    209715200,
		MaxCount:   2000,
		LicenseID:  license.ID,
	}

	response, err := service.CreateConfig(req)
	if err != nil {
		t.Fatalf("CreateConfig failed: %v", err)
	}

	if response.Name != "完整配置" {
		t.Errorf("Expected name '完整配置', got %s", response.Name)
	}
	if response.Region != "cn-hangzhou" {
		t.Errorf("Expected region 'cn-hangzhou', got %s", response.Region)
	}
	if response.Domain != "https://cdn.example.com" {
		t.Errorf("Expected domain 'https://cdn.example.com', got %s", response.Domain)
	}
	if response.MaxSize != 209715200 {
		t.Errorf("Expected max size 209715200, got %d", response.MaxSize)
	}
}

func TestObsConfigService_UpdateConfig_ChangeProvider(t *testing.T) {
	database := setupObsConfigServiceTestDB(t)
	service := newTestObsConfigService(database)

	config := &model.ObsConfig{
		Name:      "测试配置",
		Provider:  model.ObsProviderAliyun,
		AccessKey: "test-key",
		SecretKey: "test-secret",
		Bucket:    "test-bucket",
	}
	database.Create(config)

	req := &dto.UpdateObsConfigRequest{Provider: "tencent"}

	response, err := service.UpdateConfig(config.ID, req)
	if err != nil {
		t.Fatalf("UpdateConfig failed: %v", err)
	}

	if response.Provider != "tencent" {
		t.Errorf("Expected provider 'tencent', got %s", response.Provider)
	}
	if response.ProviderName != "腾讯云COS" {
		t.Errorf("Expected providerName '腾讯云COS', got %s", response.ProviderName)
	}
}

// mockFile 模拟 multipart.File 用于测试
type mockFile struct {
	content []byte
	pos     int64
}

func (m *mockFile) Read(p []byte) (n int, err error) {
	if m.pos >= int64(len(m.content)) {
		return 0, nil
	}
	n = copy(p, m.content[m.pos:])
	m.pos += int64(n)
	return n, nil
}

func (m *mockFile) ReadAt(p []byte, off int64) (n int, err error) {
	if off >= int64(len(m.content)) {
		return 0, nil
	}
	n = copy(p, m.content[off:])
	return n, nil
}

func (m *mockFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case 0:
		m.pos = offset
	case 1:
		m.pos += offset
	case 2:
		m.pos = int64(len(m.content)) + offset
	}
	return m.pos, nil
}

func (m *mockFile) Close() error {
	return nil
}
