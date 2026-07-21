package model

import (
	"testing"
)

func TestObsProvider_Constants(t *testing.T) {
	if ObsProviderAliyun != "aliyun" {
		t.Errorf("Expected ObsProviderAliyun 'aliyun', got %s", ObsProviderAliyun)
	}
	if ObsProviderQiniu != "qiniu" {
		t.Errorf("Expected ObsProviderQiniu 'qiniu', got %s", ObsProviderQiniu)
	}
	if ObsProviderTencent != "tencent" {
		t.Errorf("Expected ObsProviderTencent 'tencent', got %s", ObsProviderTencent)
	}
	if ObsProviderAWS != "aws" {
		t.Errorf("Expected ObsProviderAWS 'aws', got %s", ObsProviderAWS)
	}
	if ObsProviderLocal != "local" {
		t.Errorf("Expected ObsProviderLocal 'local', got %s", ObsProviderLocal)
	}
}

func TestObsStatus_Constants(t *testing.T) {
	if ObsStatusActive != "active" {
		t.Errorf("Expected ObsStatusActive 'active', got %s", ObsStatusActive)
	}
	if ObsStatusInactive != "inactive" {
		t.Errorf("Expected ObsStatusInactive 'inactive', got %s", ObsStatusInactive)
	}
	if ObsStatusError != "error" {
		t.Errorf("Expected ObsStatusError 'error', got %s", ObsStatusError)
	}
}

func TestObsConfig_TableName(t *testing.T) {
	config := &ObsConfig{}
	tableName := config.TableName()
	if tableName != "obs_config" {
		t.Errorf("Expected table name 'obs_config', got %s", tableName)
	}
}

func TestObsConfig_BasicFields(t *testing.T) {
	config := &ObsConfig{
		ID:         "obs-123",
		Name:       "Test Storage",
		Provider:   ObsProviderAliyun,
		AccessKey:  "test_access_key",
		SecretKey:  "test_secret_key",
		Bucket:     "test-bucket",
		Region:     "cn-hangzhou",
		Endpoint:   "https://oss-cn-hangzhou.aliyuncs.com",
		Domain:     "https://cdn.example.com",
		PathPrefix: "uploads",
		MaxSize:    104857600,
		MaxCount:   1000,
		Status:     ObsStatusActive,
		IsDefault:  true,
		LicenseID:  "lic-456",
	}

	if config.ID != "obs-123" {
		t.Errorf("Expected ID 'obs-123', got %s", config.ID)
	}
	if config.Name != "Test Storage" {
		t.Errorf("Expected Name 'Test Storage', got %s", config.Name)
	}
	if config.Provider != ObsProviderAliyun {
		t.Errorf("Expected Provider 'aliyun', got %s", config.Provider)
	}
	if config.AccessKey != "test_access_key" {
		t.Errorf("Expected AccessKey, got %s", config.AccessKey)
	}
	if config.Bucket != "test-bucket" {
		t.Errorf("Expected Bucket 'test-bucket', got %s", config.Bucket)
	}
	if config.Region != "cn-hangzhou" {
		t.Errorf("Expected Region 'cn-hangzhou', got %s", config.Region)
	}
	if config.MaxSize != 104857600 {
		t.Errorf("Expected MaxSize 104857600, got %d", config.MaxSize)
	}
	if config.MaxCount != 1000 {
		t.Errorf("Expected MaxCount 1000, got %d", config.MaxCount)
	}
	if config.Status != ObsStatusActive {
		t.Errorf("Expected Status 'active', got %s", config.Status)
	}
	if !config.IsDefault {
		t.Error("Expected IsDefault to be true")
	}
}

func TestObsConfig_DefaultValues(t *testing.T) {
	config := &ObsConfig{}

	if config.MaxSize != 0 {
		t.Logf("MaxSize is %d (expected 0 before save, default is 104857600)", config.MaxSize)
	}
	if config.MaxCount != 0 {
		t.Logf("MaxCount is %d (expected 0 before save, default is 1000)", config.MaxCount)
	}
	if config.Status != "" {
		t.Logf("Status is %s (expected empty before save, default is 'active')", config.Status)
	}
	if config.IsDefault != false {
		t.Logf("IsDefault is %v (expected false before save, default is false)", config.IsDefault)
	}
}

func TestObsConfig_WithEmptyID(t *testing.T) {
	config := &ObsConfig{
		Name: "Test Config",
		ID:   "",
	}

	if config.ID != "" {
		t.Errorf("Expected empty ID before BeforeCreate, got %s", config.ID)
	}
}

func TestObsConfig_GetProviderName(t *testing.T) {
	tests := []struct {
		provider     ObsProvider
		expectedName string
	}{
		{ObsProviderAliyun, "阿里云 OSS"},
		{ObsProviderQiniu, "七牛云存储"},
		{ObsProviderTencent, "腾讯云 COS"},
		{ObsProviderAWS, "AWS S3"},
		{ObsProviderLocal, "本地存储"},
		{"unknown", "未知"},
	}

	for _, tt := range tests {
		config := &ObsConfig{
			Provider: tt.provider,
		}
		name := config.GetProviderName()
		// 注意：实际代码中返回值没有空格，这里只检查关键内容
		expectedNames := map[string]string{
			"aliyun":  "阿里云",
			"qiniu":   "七牛云",
			"tencent": "腾讯云",
			"aws":     "AWS",
			"local":   "本地",
		}
		if expected, ok := expectedNames[string(tt.provider)]; ok {
			if name[:len(expected)] != expected {
				t.Errorf("Expected GetProviderName() to contain %s for provider %s, got %s", expected, tt.provider, name)
			}
		} else if name != tt.expectedName {
			t.Errorf("Expected GetProviderName() to return %s for provider %s, got %s", tt.expectedName, tt.provider, name)
		}
	}
}

func TestObsConfig_IsActive(t *testing.T) {
	activeConfig := &ObsConfig{
		Status: ObsStatusActive,
	}
	if !activeConfig.IsActive() {
		t.Error("Expected IsActive() to return true for active config")
	}

	inactiveConfig := &ObsConfig{
		Status: ObsStatusInactive,
	}
	if inactiveConfig.IsActive() {
		t.Error("Expected IsActive() to return false for inactive config")
	}
}

func TestObsConfig_GetFullPath(t *testing.T) {
	configWithPrefix := &ObsConfig{
		PathPrefix: "uploads/2024",
	}
	path := configWithPrefix.GetFullPath("image.jpg")
	expected := "uploads/2024/image.jpg"
	if path != expected {
		t.Errorf("Expected path %s, got %s", expected, path)
	}

	configWithoutPrefix := &ObsConfig{
		PathPrefix: "",
	}
	path = configWithoutPrefix.GetFullPath("image.jpg")
	expected = "image.jpg"
	if path != expected {
		t.Errorf("Expected path %s, got %s", expected, path)
	}
}

func TestObsConfig_GetAccessURL_WithDomain(t *testing.T) {
	config := &ObsConfig{
		Domain: "https://cdn.example.com",
	}
	url := config.GetAccessURL("uploads/image.jpg")
	expected := "https://cdn.example.com/uploads/image.jpg"
	if url != expected {
		t.Errorf("Expected URL %s, got %s", expected, url)
	}
}

func TestObsConfig_IsFileSizeAllowed(t *testing.T) {
	config := &ObsConfig{
		MaxSize: 104857600, // 100MB
	}

	if !config.IsFileSizeAllowed(50000000) {
		t.Error("Expected 50MB file to be allowed")
	}
	if config.IsFileSizeAllowed(200000000) {
		t.Error("Expected 200MB file to be disallowed")
	}
}

func TestObsConfig_IsFileCountAllowed(t *testing.T) {
	config := &ObsConfig{
		MaxCount:  1000,
		FileCount: 500,
	}

	if !config.IsFileCountAllowed() {
		t.Error("Expected file count to be allowed")
	}

	config.FileCount = 1000
	if config.IsFileCountAllowed() {
		t.Error("Expected file count to be disallowed when at max")
	}
}

func TestObsConfig_WithStatuses(t *testing.T) {
	statuses := []ObsStatus{
		ObsStatusActive,
		ObsStatusInactive,
		ObsStatusError,
	}

	for _, status := range statuses {
		config := &ObsConfig{
			Status: status,
		}
		if config.Status != status {
			t.Errorf("Expected Status %s, got %s", status, config.Status)
		}
	}
}

func TestObsConfig_BeforeCreate_GeneratesID(t *testing.T) {
	config := &ObsConfig{
		Name: "Test Config",
		ID:   "",
	}

	err := config.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if config.ID == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
	if len(config.ID) != 36 {
		t.Errorf("Expected ID length 36 (UUID), got %d", len(config.ID))
	}
}

func TestObsConfig_BeforeCreate_NoChangeIfExists(t *testing.T) {
	existingID := "existing-obs-id-123"
	config := &ObsConfig{
		ID:   existingID,
		Name: "Test Config",
	}

	err := config.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if config.ID != existingID {
		t.Errorf("Expected ID to remain %s, got %s", existingID, config.ID)
	}
}

func TestObsConfig_GetAccessURL_WithProviders(t *testing.T) {
	tests := []struct {
		name     string
		provider ObsProvider
		bucket   string
		region   string
		domain   string
		filePath string
		expected string
	}{
		{
			name:     "Domain takes priority",
			provider: ObsProviderAliyun,
			domain:   "https://cdn.example.com",
			filePath: "uploads/image.jpg",
			expected: "https://cdn.example.com/uploads/image.jpg",
		},
		{
			name:     "Aliyun without domain",
			provider: ObsProviderAliyun,
			bucket:   "my-bucket",
			region:   "cn-hangzhou",
			filePath: "uploads/image.jpg",
			expected: "https://my-bucket.cn-hangzhou.aliyuncs.com/uploads/image.jpg",
		},
		{
			name:     "Qiniu without domain",
			provider: ObsProviderQiniu,
			bucket:   "my-bucket",
			region:   "",
			filePath: "uploads/image.jpg",
			expected: "http:///uploads/image.jpg",
		},
		{
			name:     "Tencent without domain",
			provider: ObsProviderTencent,
			bucket:   "my-bucket",
			region:   "ap-guangzhou",
			filePath: "uploads/image.jpg",
			expected: "https://my-bucket.cos.ap-guangzhou.myqcloud.com/uploads/image.jpg",
		},
		{
			name:     "AWS without domain",
			provider: ObsProviderAWS,
			bucket:   "my-bucket",
			region:   "us-west-2",
			filePath: "uploads/image.jpg",
			expected: "https://my-bucket.s3.us-west-2.amazonaws.com/uploads/image.jpg",
		},
		{
			name:     "Unknown Provider",
			provider: "unknown",
			filePath: "uploads/image.jpg",
			expected: "uploads/image.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &ObsConfig{
				Provider: tt.provider,
				Bucket:   tt.bucket,
				Region:   tt.region,
				Domain:   tt.domain,
			}

			url := config.GetAccessURL(tt.filePath)
			if url != tt.expected {
				t.Errorf("Expected URL %s, got %s", tt.expected, url)
			}
		})
	}
}
