package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// CORSConfig CORS 配置
type CORSConfig struct {
	// 允许的源列表
	AllowOrigins []string `json:"allow_origins"`
	// 允许的方法
	AllowMethods []string `json:"allow_methods"`
	// 允许的头
	AllowHeaders []string `json:"allow_headers"`
	// 是否允许凭证
	AllowCredentials bool `json:"allow_credentials"`
	// 预检请求缓存时间
	MaxAge int `json:"max_age"`
	// 是否启用
	Enabled bool `json:"enabled"`
}

// DefaultCORSConfig 默认 CORS 配置
var DefaultCORSConfig = CORSConfig{
	AllowOrigins: []string{
		"http://localhost:8211",
		"http://127.0.0.1:8211",
	},
	AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
	AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-API-KEY"},
	AllowCredentials: true,
	MaxAge:           86400,
	Enabled:          true,
}

// 全局配置实例
var (
	corsConfig     CORSConfig
	corsConfigOnce sync.Once
	corsConfigMu   sync.RWMutex
)

// LoadCORSConfig 加载 CORS 配置
func LoadCORSConfig() CORSConfig {
	corsConfigOnce.Do(func() {
		configFile := getconfigFile("cors.json")
		if data, err := os.ReadFile(configFile); err == nil {
			if err := json.Unmarshal(data, &corsConfig); err != nil {
				corsConfig = DefaultCORSConfig
			}
		} else {
			corsConfig = DefaultCORSConfig
		}

		// 从环境变量覆盖配置
		if envOrigins := os.Getenv("CORS_ALLOW_ORIGINS"); envOrigins != "" {
			corsConfig.AllowOrigins = strings.Split(envOrigins, ",")
		}
		if envEnabled := os.Getenv("CORS_ENABLED"); envEnabled == "false" {
			corsConfig.Enabled = false
		}
	})

	corsConfigMu.RLock()
	defer corsConfigMu.RUnlock()
	return corsConfig
}

// GetCORSOrigins 获取允许的源列表。
// 基于本项目部署特性（内网局域网 / 自有站点演示 / 开发环境），
// 不限制跨域来源，允许所有域名。
func GetCORSOrigins() []string {
	return []string{"*"}
}

// GetCORSMethods 获取允许的方法
func GetCORSMethods() []string {
	corsConfigMu.RLock()
	defer corsConfigMu.RUnlock()
	return corsConfig.AllowMethods
}

// GetCORSHeaders 获取允许的头
func GetCORSHeaders() []string {
	corsConfigMu.RLock()
	defer corsConfigMu.RUnlock()
	return corsConfig.AllowHeaders
}

// IsCORSEnabled 检查 CORS 是否启用
func IsCORSEnabled() bool {
	corsConfigMu.RLock()
	defer corsConfigMu.RUnlock()
	return corsConfig.Enabled
}

// IsCredentialsAllowed 检查是否允许凭证
func IsCredentialsAllowed() bool {
	corsConfigMu.RLock()
	defer corsConfigMu.RUnlock()
	return corsConfig.AllowCredentials
}

// GetCORSMaxAge 获取预检请求缓存时间
func GetCORSMaxAge() int {
	corsConfigMu.RLock()
	defer corsConfigMu.RUnlock()
	return corsConfig.MaxAge
}

// SaveCORSConfig 保存 CORS 配置
func SaveCORSConfig(config CORSConfig) error {
	corsConfigMu.Lock()
	defer corsConfigMu.Unlock()

	configFile := getconfigFile("cors.json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	// 使用安全的文件权限
	return os.WriteFile(configFile, data, 0600)
}

// contains 检查切片是否包含某元素
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// getconfigFile 获取配置文件路径
func getconfigFile(filename string) string {
	envDir := GetEnvDir()
	return filepath.Join(envDir, filename)
}
