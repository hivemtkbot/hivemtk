package middleware

import (
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/system/install"
	"sync"
)

// LicenseChecker 初始化检查器（开源版精简版）
//
// 背景：hivemtk 全面开源后不再有 License / 授权流程，
// 该类型仅承载 install.lock 状态查询 + 心跳相关统计，
// 真正"是否已初始化"的判断依据 install.lock.Initialized == true。
//
// 兼容老代码：保留旧接口签名（GetInitStatus / HasInstallLockAdmin /
// SetAdminInit / GetInstallLock / CurrentVersion / GetAdminUsername / MarkAdminInitialized），
// 但内部已不再请求平台获取授权。
type LicenseChecker struct {
	// ServerURL 平台上报地址（保留字段以兼容 InitLicenseChecker(serverURL, licenseKey) 调用方）
	// 开源版实际不再用于授权校验，仅作为心跳 / 安装上报的 base_url。
	ServerURL string
	// LicenseKey 保留字段，调用方传 "" 即可，开源版不校验
	LicenseKey string

	// mu 保护并发：标记初始化时
	mu sync.Mutex
}

var (
	globalChecker *LicenseChecker
	globalMu      sync.RWMutex
)

// InitLicenseChecker 初始化全局 LicenseChecker（开源版：仅记录地址，不再做授权校验）
//
// 参数：
//   - serverURL  平台上报地址（用于心跳 / 安装信息上报）
//   - licenseKey 兼容旧接口，开源版忽略（传 "" 即可）
func InitLicenseChecker(serverURL, licenseKey string) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalChecker = &LicenseChecker{
		ServerURL:  serverURL,
		LicenseKey: licenseKey,
	}
	logger.Infof("[InitLicenseChecker] open-source edition: server_url=%s (license removed)", serverURL)
}

// GetLicenseChecker 返回全局 LicenseChecker，未初始化时返回 nil
func GetLicenseChecker() *LicenseChecker {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalChecker
}

// SetServerURL 更新全局平台地址（兼容老代码）
func (c *LicenseChecker) SetServerURL(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ServerURL = url
}

// GetInitStatus 返回初始化状态 DTO
func (c *LicenseChecker) GetInitStatus() *install.Status {
	if c == nil {
		return &install.Status{State: "NOT_INSTALLED", Initialized: false, HasAdmin: false}
	}
	return install.GetStatus()
}

// HasInstallLockAdmin 判定 install.lock 是否已记录超管账号
func (c *LicenseChecker) HasInstallLockAdmin() bool {
	return install.GetAdminUsername() != ""
}

// SetAdminInit 写入 install.lock 并标记为 INITIALIZED
func (c *LicenseChecker) SetAdminInit(username string) error {
	return install.MarkAdminInitialized(username)
}

// GetInstallLock 返回当前 install.lock（拷贝）
func (c *LicenseChecker) GetInstallLock() *install.Lock {
	lr, err := install.Load()
	if err != nil || lr == nil {
		return nil
	}
	return lr
}

// CurrentVersion 返回 install.lock 记录的版本号；缺失时返回 unknown
func (c *LicenseChecker) CurrentVersion() string {
	lr, err := install.Load()
	if err != nil || lr == nil {
		return "unknown"
	}
	if lr.Version == "" {
		return "unknown"
	}
	return lr.Version
}

// GetAdminUsername 返回 install.lock 中的超管账号
func (c *LicenseChecker) GetAdminUsername() string {
	return install.GetAdminUsername()
}

// MarkAdminInitialized 兼容老接口：标记 install.lock 已初始化
func (c *LicenseChecker) MarkAdminInitialized() error {
	return install.MarkAdminInitializedStandalone()
}
