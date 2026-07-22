package dto

import "time"

// ObsConfigRequest OBS配置请求
//
// 开源版：已移除 LicenseID 字段（License 模型已删除，授权流程下线）。
type CreateObsConfigRequest struct {
	Name       string `json:"name" binding:"required"`
	Provider   string `json:"provider" binding:"required,oneof=aliyun qiniu tencent aws local"`
	AccessKey  string `json:"access_key" binding:"required"`
	SecretKey  string `json:"secret_key" binding:"required"`
	Bucket     string `json:"bucket" binding:"required"`
	Region     string `json:"region"`
	Endpoint   string `json:"endpoint"`
	Domain     string `json:"domain"`
	PathPrefix string `json:"path_prefix"`
	Config     string `json:"config"`
	MaxSize    int64  `json:"max_size"`
	MaxCount   int    `json:"max_count"`
}

type UpdateObsConfigRequest struct {
	Name       string `json:"name"`
	Provider   string `json:"provider" binding:"omitempty,oneof=aliyun qiniu tencent aws local"`
	AccessKey  string `json:"access_key"`
	SecretKey  string `json:"secret_key"`
	Bucket     string `json:"bucket"`
	Region     string `json:"region"`
	Endpoint   string `json:"endpoint"`
	Domain     string `json:"domain"`
	PathPrefix string `json:"path_prefix"`
	Config     string `json:"config"`
	MaxSize    int64  `json:"max_size"`
	MaxCount   int    `json:"max_count"`
}

// ObsConfigResponse OBS配置响应
//
// 开源版：已移除 LicenseID 字段。
type ObsConfigResponse struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Provider     string     `json:"provider"`
	ProviderName string     `json:"provider_name"`
	AccessKey    string     `json:"access_key"`
	SecretKey    string     `json:"secret_key"` // 返回掩码
	Bucket       string     `json:"bucket"`
	Region       string     `json:"region"`
	Endpoint     string     `json:"endpoint"`
	Domain       string     `json:"domain"`
	PathPrefix   string     `json:"path_prefix"`
	Config       string     `json:"config"`
	MaxSize      int64      `json:"max_size"`
	MaxCount     int        `json:"max_count"`
	Status       string     `json:"status"`
	LastError    string     `json:"last_error"`
	LastTestAt   *time.Time `json:"last_test_at"`
	TotalSize    int64      `json:"total_size"`
	FileCount    int        `json:"file_count"`
	IsDefault    bool       `json:"is_default"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// GetObsConfigListResponse OBS配置列表响应
type GetObsConfigListResponse struct {
	List  []*ObsConfigResponse `json:"list"`
	Total int64                `json:"total"`
	Page  int                  `json:"page"`
	Limit int                  `json:"limit"`
}

// TestConnectionRequest 连接测试请求
type TestConnectionRequest struct {
	ID string `json:"id" binding:"required"`
}

// SetDefaultRequest 设置默认配置请求
type SetDefaultRequest struct {
	ID string `json:"id" binding:"required"`
}
