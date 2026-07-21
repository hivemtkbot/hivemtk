package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// AuthResponse 授权响应结构
type AuthResponse struct {
	Authorized bool   `json:"authorized"`
	Message    string `json:"message"`
}

// CacheEntry 缓存条目结构
type CacheEntry struct {
	Authorized bool      `json:"authorized"`
	Expiry     time.Time `json:"expiry"`
}

// APIKeyInfo API Key 信息
type APIKeyInfo struct {
	Key string `json:"key"`

	Status    int       `json:"status"` // 1=active, 0=inactive
	ExpiresAt time.Time `json:"expires_at"`
}

// 全局缓存变量
var (
	authCache      = make(map[string]CacheEntry)
	authCacheMutex sync.RWMutex
	apiKeyCache    = make(map[string]*APIKeyInfo)
	apiKeyMutex    sync.RWMutex
)

// 授权检查 URL
const authCheckURL = "https://auth.xapptool.cn"

// 缓存文件路径
var cacheFilePath string

// 常量定义
const (
	// 缓存过期时间
	cacheExpiryDuration = 5 * time.Minute
	// API Key 缓存过期时间
	apiKeyCacheExpiry = 10 * time.Minute
	// 授权超时时间
	authTimeout = 5 * time.Second
)

// 初始化缓存文件路径
func init() {
	// 获取当前执行文件目录
	exePath, err := os.Executable()
	if err != nil {
		// 如果获取失败，使用当前工作目录
		exePath, _ = os.Getwd()
	}

	// 获取目录路径
	dir := filepath.Dir(exePath)

	// 创建缓存文件路径
	cacheFilePath = filepath.Join(dir, "auth_cache.json")

	// 尝试加载已有缓存
	loadCacheFromFile()
}

// AuthMiddleware API 鉴权中间件 - 修复版本
// 安全加固：
// 1. 不再简单地检查 API Key 是否为空
// 2. 实现本地 API Key 验证逻辑
// 3. 故障时默认拒绝而非放行
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取请求头中的 API Key
		apiKey := c.GetHeader("X-API-KEY")

		// API Key 为空，直接返回未授权
		if apiKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code": http.StatusUnauthorized,
				"msg":  "未授权，请提供有效的 API Key",
			})
			c.Abort()
			return
		}

		// 验证 API Key 格式（基本校验）
		if len(apiKey) < 16 || !isValidAPIKeyFormat(apiKey) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code": http.StatusUnauthorized,
				"msg":  "无效的 API Key 格式",
			})
			c.Abort()
			return
		}

		// 验证 API Key 有效性
		if !validateAPIKey(apiKey) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code": http.StatusUnauthorized,
				"msg":  "API Key 无效或已过期",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// isValidAPIKeyFormat 验证 API Key 格式
// 要求：至少 16 位，包含字母和数字
func isValidAPIKeyFormat(apiKey string) bool {
	hasLetter := false
	hasDigit := false
	for _, r := range apiKey {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			hasLetter = true
		}
		if r >= '0' && r <= '9' {
			hasDigit = true
		}
		if hasLetter && hasDigit {
			return true
		}
	}
	return false
}

// validateAPIKey 验证 API Key 有效性
func validateAPIKey(apiKey string) bool {
	// 1. 先检查本地缓存
	if info, found := getAPIKeyFromCache(apiKey); found {
		return isAPIKeyValid(info)
	}

	// 2. 缓存未命中，检查授权服务
	authorized, err := checkAuthorizationWithTimeout(apiKey)
	if err != nil {
		// 故障安全：外部服务不可用时，使用降级策略
		// 检查是否是测试/开发环境
		if isDevOrTestMode() {
			return true
		}
		// 生产环境：故障时拒绝访问
		return false
	}

	// 3. 缓存结果
	if authorized {
		cacheAPIKey(apiKey, &APIKeyInfo{
			Key:    apiKey,
			Status: 1,
		})
	}

	return authorized
}

// checkAuthorizationWithTimeout 带超时的授权检查
func checkAuthorizationWithTimeout(apiKey string) (bool, error) {
	// 使用更短的超时时间
	ctx, cancel := context.WithTimeout(context.Background(), authTimeout)
	defer cancel()

	// 创建请求体
	requestData := map[string]any{
		"api_key": apiKey,
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return false, fmt.Errorf("序列化请求数据失败：%v", err)
	}

	// 创建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, "POST", authCheckURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return false, fmt.Errorf("创建请求失败：%v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Marketing-System/1.0")

	// 发送请求
	client := &http.Client{Timeout: authTimeout}
	resp, err := client.Do(req)
	if err != nil {
		// 网络错误，返回错误
		return false, fmt.Errorf("授权服务请求失败：%v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("读取响应失败：%v", err)
	}

	// 解析响应
	var authResp AuthResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		// 解析失败返回错误，不再默认放行
		return false, fmt.Errorf("解析授权响应失败：%v", err)
	}

	return authResp.Authorized, nil
}

// isAPIKeyValid 检查 API Key 是否有效
func isAPIKeyValid(info *APIKeyInfo) bool {
	if info.Status != 1 {
		return false
	}
	// 检查是否过期
	if !info.ExpiresAt.IsZero() && time.Now().After(info.ExpiresAt) {
		return false
	}
	return true
}

// getAPIKeyFromCache 从缓存获取 API Key 信息
func getAPIKeyFromCache(apiKey string) (*APIKeyInfo, bool) {
	apiKeyMutex.RLock()
	defer apiKeyMutex.RUnlock()
	info, found := apiKeyCache[apiKey]
	return info, found
}

// cacheAPIKey 缓存 API Key 信息
func cacheAPIKey(apiKey string, info *APIKeyInfo) {
	apiKeyMutex.Lock()
	defer apiKeyMutex.Unlock()
	apiKeyCache[apiKey] = info
}

// isDevOrTestMode 检查是否是开发或测试模式
func isDevOrTestMode() bool {
	return os.Getenv("IS_TEST_MODE") == "true" ||
		os.Getenv("GIN_MODE") == "debug" ||
		os.Getenv("APP_ENV") == "development"
}

// loadCacheFromFile 从文件加载缓存
func loadCacheFromFile() {
	authCacheMutex.Lock()
	defer authCacheMutex.Unlock()

	data, err := os.ReadFile(cacheFilePath)
	if err != nil {
		return
	}

	authCache = make(map[string]CacheEntry)
	if err := json.Unmarshal(data, &authCache); err != nil {
		return
	}

	// 清理过期缓存
	now := time.Now()
	for key, entry := range authCache {
		if now.After(entry.Expiry) {
			delete(authCache, key)
		}
	}
}

// saveCacheToFile 保存缓存到文件
func saveCacheToFile() {
	authCacheMutex.Lock()
	defer authCacheMutex.Unlock()

	// 清理过期缓存
	now := time.Now()
	for key, entry := range authCache {
		if now.After(entry.Expiry) {
			delete(authCache, key)
		}
	}

	data, err := json.MarshalIndent(authCache, "", "  ")
	if err != nil {
		return
	}

	// 使用安全的文件权限（仅所有者可读写）
	if err := os.WriteFile(cacheFilePath, data, 0600); err != nil {
		return
	}
}

// ClearAuthCache 清空授权缓存（用于测试或重置）
func ClearAuthCache() {
	authCacheMutex.Lock()
	apiKeyMutex.Lock()
	defer authCacheMutex.Unlock()
	defer apiKeyMutex.Unlock()

	authCache = make(map[string]CacheEntry)
	apiKeyCache = make(map[string]*APIKeyInfo)
	os.Remove(cacheFilePath)
}

// Import API Key 相关函数供其他包使用
// ValidateAPIKey 公开方法供外部调用
func ValidateAPIKey(apiKey string) bool {
	return validateAPIKey(apiKey)
}
