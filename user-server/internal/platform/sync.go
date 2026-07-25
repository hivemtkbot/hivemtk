package platform

import (
	"crypto/rand"
	"encoding/hex"
	"runtime"

	"marketing/internal/pkg/utils/logger"
)

// merchantKey 单租户独立部署的本地标识（用于 platform 包内 API 调用的身份标记）
var merchantKey string

// generateRandomKey 生成指定长度的随机 key
func generateRandomKey(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	key := make([]byte, length)
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString(b[:length])
	}
	for i := range key {
		key[i] = charset[int(b[i])%len(charset)]
	}
	return string(key)
}

// InitSync 初始化平台同步
// 独立部署版本：商户端为单租户独立部署，不再向平台上报日志或同步授权。
func InitSync() error {
	merchantKey = generateRandomKey(32)
	logger.Info("[独立部署模式] 平台同步已禁用（InitSync no-op）")
	return nil
}

// StopAllTasks 停止所有后台任务
// 独立部署版本：无需停止任何平台同步任务，保留为 no-op 以保持外部调用方不变。
func StopAllTasks() {}

// RegisterMerchant 向平台注册本地商户信息
func RegisterMerchant(req RegisterMerchantReq) error {
	if req.DeviceInfo == "" {
		req.DeviceInfo = runtime.GOOS + " " + runtime.GOARCH
	}
	cli := NewPlatformClient(merchantKey)
	return cli.RegisterMerchant(req)
}

// GetLicenseStatus 获取授权状态
func GetLicenseStatus() (*LicenseStatusResp, error) {
	cli := NewPlatformClient(merchantKey)
	return cli.GetLicenseStatus()
}

// GetMerchantKey 返回当前部署实例的 merchant key
func GetMerchantKey() string {
	return merchantKey
}

// SetMerchantKeyForTest 测试时设置 merchant key
func SetMerchantKeyForTest(key string) {
	merchantKey = key
}
