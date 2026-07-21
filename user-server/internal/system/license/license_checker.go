package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"marketing/internal/config"
	"marketing/internal/pkg/utils/logger"
	"net/http"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// 授权错误
var (
	ErrLicenseMissing           = fmt.Errorf("license file missing")
	ErrLicenseExpired           = fmt.Errorf("license expired")
	ErrLicenseSuspended         = fmt.Errorf("license suspended")
	ErrLicenseRevoked           = fmt.Errorf("license revoked")
	ErrLicenseServerUnreachable = fmt.Errorf("license server unreachable")
	ErrLicenseInvalid           = fmt.Errorf("license invalid")
)

// LicenseCheckerConfig 授权检查器配置
type LicenseCheckerConfig struct {
	LicenseFile    string        // install.lock 路径
	Interval       time.Duration // 心跳间隔，默认 3 分钟
	Timeout        time.Duration // HTTP 请求超时，默认 10 秒
	GraceCount     int           // 允许失败次数，默认 3 次（9 分钟）
	ServerURL      string        // 平台授权服务端 URL
	CurrentVersion string        // 当前版本号
	PlatformSecret string        // 平台共享密钥，用于 install.lock HMAC 签名校验
}

// DefaultLicenseCheckerConfig 默认配置（对应 LICENSE_OTA_DESIGN.md）
func DefaultLicenseCheckerConfig() LicenseCheckerConfig {
	return LicenseCheckerConfig{
		LicenseFile:    getInstallLockPath(),
		Interval:       3 * time.Minute,
		Timeout:        10 * time.Second,
		GraceCount:     3,
		ServerURL:      getPlatformURL(),
		CurrentVersion: "1.0.0",
	}
}

// getInstallLockPath 获取 install.lock 路径
// 优先级: 环境变量 INSTALL_LOCK_PATH > /opt/mtk/install.lock > ./install.lock
func getInstallLockPath() string {
	return GetInstallLockPath()
}

// GetInstallLockPath 公开版本：供 controller / service 直接调用
// 优先级: 环境变量 INSTALL_LOCK_PATH > /opt/mtk/install.lock > ./install.lock
func GetInstallLockPath() string {
	if p := os.Getenv("INSTALL_LOCK_PATH"); p != "" {
		return p
	}
	// 生产路径
	if _, err := os.Stat("/opt/mtk/install.lock"); err == nil {
		return "/opt/mtk/install.lock"
	}
	// 开发路径
	return "install.lock"
}

// DefaultPlatformAPI 开源版默认平台上报域名
// 对应需求：hivemtk 初始化后直接将安装信息上报到该平台。
const DefaultPlatformAPI = "https://hivepaltformapi.xapptool.cn"

// getPlatformURL 获取平台 URL
func getPlatformURL() string {
	if config.PlatformCfg != nil && config.PlatformCfg.APIURL != "" {
		return config.PlatformCfg.APIURL
	}
	if u := os.Getenv("PLATFORM_URL"); u != "" {
		return u
	}
	return DefaultPlatformAPI
}

// LicenseStatus 授权状态
type LicenseStatus string

const (
	LicenseStatusActive    LicenseStatus = "active"
	LicenseStatusSuspended LicenseStatus = "suspended"
	LicenseStatusExpired   LicenseStatus = "expired"
	LicenseStatusRevoked   LicenseStatus = "revoked"
	LicenseStatusOffline   LicenseStatus = "offline"
	LicenseStatusMissing   LicenseStatus = "missing"
	LicenseStatusInvalid   LicenseStatus = "invalid" // 签名/格式不合法
)

// LicenseChecker 授权检查器
// 对应 LICENSE_OTA_DESIGN.md 第 3.4-3.6 节
type LicenseChecker struct {
	cfg          LicenseCheckerConfig
	httpClient   *http.Client
	installLock  *InstallLock
	graceCount   int32        // 原子操作，允许失败次数
	status       atomic.Value // LicenseStatus
	lastCheckAt  time.Time
	lastCheckErr error
	checkMu      sync.Mutex  // 保护 lastCheckAt/lastCheckErr 的并发访问
	checking     atomic.Bool // 防止 Check() goroutine 堆积
	heartbeating atomic.Bool // 防止 SendHeartbeat() goroutine 堆积
	mu           sync.RWMutex
	shutdownFunc func() // 优雅停机回调
}

// NewLicenseChecker 创建新的授权检查器
func NewLicenseChecker(cfg LicenseCheckerConfig) *LicenseChecker {
	if cfg.Interval <= 0 {
		cfg.Interval = 3 * time.Minute
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.GraceCount <= 0 {
		cfg.GraceCount = 3
	}
	lc := &LicenseChecker{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}
	lc.graceCount = int32(cfg.GraceCount)
	lc.status.Store(LicenseStatusMissing)
	return lc
}

// SetShutdownFunc 设置优雅停机回调
func (lc *LicenseChecker) SetShutdownFunc(fn func()) {
	lc.shutdownFunc = fn
}

// LoadInstallLock 加载本地 install.lock
// 关键：模板文件（install.lock.example）无签名，必须能正确识别为"未初始化"状态
func (lc *LicenseChecker) LoadInstallLock() error {
	data, err := os.ReadFile(lc.cfg.LicenseFile)
	if err != nil {
		if os.IsNotExist(err) {
			lc.status.Store(LicenseStatusMissing)
			return ErrLicenseMissing
		}
		return fmt.Errorf("读取 install.lock 失败: %w", err)
	}

	lock, err := UnmarshalInstallLock(data)
	if err != nil {
		return fmt.Errorf("解析 install.lock 失败: %w", err)
	}

	// 模板文件/空 install.lock 判定（开源版）：仅当 LicenseKey、InstallID、AdminUsername
	// 全部为空时才视为"未初始化"模板；否则视为真实安装记录（即使无 LicenseKey，
	// 即开源模式下 install.lock 仅承载 install_id / 管理员信息 / 安装信息，不再需要 LicenseKey）。
	if lock.LicenseKey == "" && lock.InstallID == "" && lock.AdminUsername == "" {
		lc.status.Store(LicenseStatusMissing)
		lc.mu.Lock()
		lc.installLock = nil // 不持有模板文件，避免后续误判
		lc.mu.Unlock()
		return ErrLicenseMissing
	}

	// 有 LicenseKey 的 install.lock 必须通过签名校验（防本地篡改）
	// 注意：若未配置 PlatformSecret，则跳过校验（开发模式/单机部署兼容）
	// 开源版：无 LicenseKey 时不做签名校验（install.lock 不再承载授权信息）
	if lock.LicenseKey != "" && lc.cfg.PlatformSecret != "" {
		if err := lock.VerifySignature(lc.cfg.PlatformSecret); err != nil {
			// 合法遗留锁（如旧版 BindLicenseInit 未签名）自动补签名，
			// 避免授权在初始化或容器重启后被误判为失效（LicenseStatusInvalid）。
			// 仅当签名字段为空（从未签名）时自动补签；若签名存在但校验失败，
			// 视为真实篡改，仍判定为无效。
			if lock.Signature == "" {
				if sErr := lock.Sign(lc.cfg.PlatformSecret); sErr == nil {
					if data, mErr := lock.Marshal(); mErr == nil {
						_ = os.WriteFile(lc.cfg.LicenseFile, data, 0600)
					}
					logger.Info("install.lock 签名缺失，已自动补签并落盘")
				}
			} else {
				lc.status.Store(LicenseStatusInvalid)
				return fmt.Errorf("install.lock 签名验证失败: %w", err)
			}
		}
	} else {
		logger.Info("PlatformSecret 未配置，跳过 install.lock 签名校验（开发模式）")
	}

	lc.mu.Lock()
	lc.installLock = lock
	lc.mu.Unlock()

	// 本地过期检查
	if lock.IsExpired() {
		lc.status.Store(LicenseStatusExpired)
		return ErrLicenseExpired
	}

	lc.status.Store(LicenseStatusActive)
	return nil
}

// EnsureInstallID 确保有 install_id，没有则生成并写入
func (lc *LicenseChecker) EnsureInstallID() error {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if lc.installLock == nil {
		return ErrLicenseMissing
	}

	if lc.installLock.InstallID == "" {
		lc.installLock.InstallID = "inst-" + uuid.New().String()
		// 写回文件
		return lc.writeInstallLockLocked()
	}
	return nil
}

// writeInstallLockLocked 写入 install.lock（调用方需持有锁）
// 写入前用 PlatformSecret 重新签名，保证签名与 payload 始终一致（防本地篡改）。
// 这是所有 install.lock 写操作的唯一出口：BindLicense / BindLicenseInit /
// SetAdminInit / MarkAdminInitialized / EnsureInstallID 均经此落盘。
func (lc *LicenseChecker) writeInstallLockLocked() error {
	if lc.cfg.PlatformSecret != "" && lc.installLock != nil {
		if err := lc.installLock.Sign(lc.cfg.PlatformSecret); err != nil {
			return fmt.Errorf("签名 install.lock 失败: %w", err)
		}
	}
	data, err := lc.installLock.Marshal()
	if err != nil {
		return fmt.Errorf("序列化 install.lock 失败: %w", err)
	}
	if err := os.WriteFile(lc.cfg.LicenseFile, data, 0600); err != nil {
		return fmt.Errorf("写入 install.lock 失败: %w", err)
	}
	return nil
}

// SaveInstallLock 保存 install.lock
func (lc *LicenseChecker) SaveInstallLock(lock *InstallLock) error {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	lc.installLock = lock
	if lock.InstallID == "" {
		lock.InstallID = "inst-" + uuid.New().String()
	}
	return lc.writeInstallLockLocked()
}

// BindLicense 绑定授权码（首次安装或换绑）
// 调用平台 /api/platform/auth/verify 验证授权码，成功后写入 install.lock
func (lc *LicenseChecker) BindLicense(licenseKey string) (*InstallLock, error) {
	// 调用平台验证
	verifyResp, err := lc.callPlatformVerify(licenseKey)
	if err != nil {
		return nil, fmt.Errorf("平台验证失败: %w", err)
	}
	if !verifyResp.Data.Valid {
		return nil, fmt.Errorf("授权码无效: %s", verifyResp.Data.Reason)
	}

	// 构造 install.lock（保留已有 install_id/AdminUsername 等字段）
	lc.mu.Lock()
	existing := lc.installLock
	installID := "inst-" + uuid.New().String()
	if existing != nil && existing.InstallID != "" {
		installID = existing.InstallID
	}
	adminUsername := ""
	adminInitialized := false
	var initializedAt *time.Time
	if existing != nil {
		adminUsername = existing.AdminUsername
		adminInitialized = existing.AdminInitialized
		initializedAt = existing.InitializedAt
	}
	lc.mu.Unlock()

	lock := &InstallLock{
		LicenseKey:       verifyResp.Data.LicenseKey,
		InstallID:        installID,
		Company:          verifyResp.Data.Company,
		ContactEmail:     "",
		IssuedAt:         time.Now(),
		ExpiresAt:        verifyResp.Data.ExpiresAt,
		Trial:            verifyResp.Data.Trial,
		MaxUsers:         0,          // 私域部署:不限制用户数
		Features:         []string{}, // 私域部署:不限制功能
		Version:          lc.cfg.CurrentVersion,
		AdminUsername:    adminUsername,
		AdminInitialized: adminInitialized,
		InitializedAt:    initializedAt,
	}

	if err := lc.SaveInstallLock(lock); err != nil {
		return nil, err
	}

	lc.status.Store(LicenseStatusActive)
	atomic.StoreInt32(&lc.graceCount, int32(lc.cfg.GraceCount))
	return lock, nil
}

// Check 执行一次授权检查
// 对应 LICENSE_OTA_DESIGN.md 第 3.4 节 Check()
func (lc *LicenseChecker) Check() error {
	// 1. 读取本地授权文件
	lc.mu.RLock()
	localLock := lc.installLock
	lc.mu.RUnlock()

	if localLock == nil {
		if err := lc.LoadInstallLock(); err != nil {
			lc.recordCheckResult(err)
			return err
		}
		lc.mu.RLock()
		localLock = lc.installLock
		lc.mu.RUnlock()
	}

	if localLock == nil {
		lc.status.Store(LicenseStatusMissing)
		lc.recordCheckResult(ErrLicenseMissing)
		return ErrLicenseMissing
	}

	// 2. 检查本地过期时间（无需联网）
	if localLock.IsExpired() {
		lc.status.Store(LicenseStatusExpired)
		lc.recordCheckResult(ErrLicenseExpired)
		return ErrLicenseExpired
	}

	// 开源版：无 LicenseKey 时跳过联网授权校验（授权逻辑已移除），
	// 仅保留本地统计上报（心跳），不再做任何授权相关联网校验 / 进程退出。
	if localLock.LicenseKey == "" {
		lc.status.Store(LicenseStatusActive)
		lc.recordCheckResult(nil)
		return nil
	}

	// 3. 调用平台 API 验证（带容错）
	verifyResp, err := lc.callPlatformVerify(localLock.LicenseKey)
	if err != nil {
		// 网络错误，容错处理
		remaining := atomic.AddInt32(&lc.graceCount, -1)
		lc.recordCheckResult(err)
		if remaining <= 0 {
			lc.status.Store(LicenseStatusOffline)
			return ErrLicenseServerUnreachable
		}
		// 容错期内放行
		logger.Info(fmt.Sprintf("平台验证失败，进入容错期，剩余次数: %d", remaining))
		return nil
	}

	// 4. 平台返回的状态检查
	if !verifyResp.Data.Valid {
		errInvalid := fmt.Errorf("授权无效: %s", verifyResp.Data.Reason)
		lc.recordCheckResult(errInvalid)
		switch verifyResp.Data.Status {
		case "suspended":
			lc.status.Store(LicenseStatusSuspended)
			return ErrLicenseSuspended
		case "revoked":
			lc.status.Store(LicenseStatusRevoked)
			return ErrLicenseRevoked
		case "expired":
			lc.status.Store(LicenseStatusExpired)
			return ErrLicenseExpired
		default:
			lc.status.Store(LicenseStatusSuspended)
			return ErrLicenseInvalid
		}
	}

	// 5. 重置失败计数
	atomic.StoreInt32(&lc.graceCount, int32(lc.cfg.GraceCount))
	lc.status.Store(LicenseStatusActive)
	lc.recordCheckResult(nil)

	// 6. 同步远程过期时间到本地（如果远程更严格）
	lc.mu.Lock()
	if lc.installLock != nil && verifyResp.Data.ExpiresAt.Before(lc.installLock.ExpiresAt) {
		lc.installLock.ExpiresAt = verifyResp.Data.ExpiresAt
		_ = lc.writeInstallLockLocked()
	}
	lc.mu.Unlock()

	return nil
}

// Run 启动心跳检测循环
// 对应 LICENSE_OTA_DESIGN.md 第 3.5 节
func (lc *LicenseChecker) Run(ctx context.Context) {
	ticker := time.NewTicker(lc.cfg.Interval)
	defer ticker.Stop()

	// 启动时立即检查一次
	// 开源版：授权检查不再触发进程退出（Shutdown）。授权逻辑已移除，
	// 检查仅用于统计/心跳上报，任何错误仅记录日志，不影响业务运行。
	if err := lc.Check(); err != nil {
		logger.Warn(fmt.Sprintf("授权检查（统计上报）初始失败，已忽略: %v", err))
	}

	// 同时上报心跳
	_ = lc.SendHeartbeat()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 并行：授权检查 + 心跳上报
			// 使用 atomic 标志位防止 goroutine 堆积：上一次未完成则跳过本次
			if lc.checking.CompareAndSwap(false, true) {
				go func() {
					defer lc.checking.Store(false)
					_ = lc.Check()
				}()
			}
			if lc.heartbeating.CompareAndSwap(false, true) {
				go func() {
					defer lc.heartbeating.Store(false)
					_ = lc.SendHeartbeat()
				}()
			}
		}
	}
}

// QuickCheck 快速检查（中间件使用，不发起网络请求）
// 对应 LICENSE_OTA_DESIGN.md 第 7.2 节 LicenseGuard.QuickCheck
//
// 私域独立部署基线：本地 install.lock 是"信任锚"——只要本地 lock
// 通过签名校验且未过期，即使无法连接 platform-server 也必须放行。
// 这样断网场景（platform 暂时不可达）不会阻断商户自部署系统的业务。
func (lc *LicenseChecker) QuickCheck() error {
	status := lc.Status()
	switch status {
	case LicenseStatusActive:
		return nil
	case LicenseStatusOffline:
		// 私域独立部署：只要本地 lock 仍有效（签名通过 + 未过期），
		// 即使 graceCount 归零也放行——这是断网容错的正确语义
		lc.mu.RLock()
		lock := lc.installLock
		lc.mu.RUnlock()
		if lock != nil && !lock.IsExpired() {
			return nil
		}
		// 本地 lock 已过期或缺失，走传统容错逻辑
		if atomic.LoadInt32(&lc.graceCount) > 0 {
			return nil
		}
		return ErrLicenseServerUnreachable
	case LicenseStatusSuspended:
		return ErrLicenseSuspended
	case LicenseStatusExpired:
		return ErrLicenseExpired
	case LicenseStatusRevoked:
		return ErrLicenseRevoked
	default:
		return ErrLicenseMissing
	}
}

// Shutdown 优雅停机
// 对应 LICENSE_OTA_DESIGN.md 第 3.6 节
func (lc *LicenseChecker) Shutdown() {
	logger.Warn("授权无效，开始优雅停机...")

	if lc.shutdownFunc != nil {
		lc.shutdownFunc()
	}
}

// Status 获取当前授权状态
func (lc *LicenseChecker) Status() LicenseStatus {
	return lc.status.Load().(LicenseStatus)
}

// recordCheckResult 记录本次检查结果（并发安全）
func (lc *LicenseChecker) recordCheckResult(err error) {
	lc.checkMu.Lock()
	lc.lastCheckAt = time.Now()
	lc.lastCheckErr = err
	lc.checkMu.Unlock()
}

// LastCheckAt 获取上次检查时间（并发安全）
func (lc *LicenseChecker) LastCheckAt() time.Time {
	lc.checkMu.Lock()
	defer lc.checkMu.Unlock()
	return lc.lastCheckAt
}

// LastCheckErr 获取上次检查错误（并发安全）
func (lc *LicenseChecker) LastCheckErr() error {
	lc.checkMu.Lock()
	defer lc.checkMu.Unlock()
	return lc.lastCheckErr
}

// ExpiresAt 获取过期时间
func (lc *LicenseChecker) ExpiresAt() time.Time {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	if lc.installLock == nil {
		return time.Time{}
	}
	return lc.installLock.ExpiresAt
}

// GetInstallLock 获取 install.lock（只读）
func (lc *LicenseChecker) GetInstallLock() *InstallLock {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	if lc.installLock == nil {
		return nil
	}
	// 返回副本
	dup := *lc.installLock
	return &dup
}

// IsLicensed 是否已授权
// IsLicensed 是否已授权
// 开源版：授权逻辑已移除，始终返回 true（不再有任何授权拦截）。
func (lc *LicenseChecker) IsLicensed() bool {
	return true
}

// CurrentVersion 返回当前部署版本号（用于安装信息上报）。
func (lc *LicenseChecker) CurrentVersion() string {
	return lc.cfg.CurrentVersion
}

// UnbindLicense 解绑授权码（删除 install.lock）
func (lc *LicenseChecker) UnbindLicense() error {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if err := os.Remove(lc.cfg.LicenseFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除 install.lock 失败: %w", err)
	}

	lc.installLock = nil
	lc.status.Store(LicenseStatusMissing)
	atomic.StoreInt32(&lc.graceCount, int32(lc.cfg.GraceCount))
	return nil
}

// VerifyLicense 验证授权码（不写入 install.lock）
func (lc *LicenseChecker) VerifyLicense(licenseKey string) (*InstallLock, error) {
	verifyResp, err := lc.callPlatformVerify(licenseKey)
	if err != nil {
		return nil, fmt.Errorf("平台验证失败: %w", err)
	}
	if !verifyResp.Data.Valid {
		return nil, fmt.Errorf("授权码无效: %s", verifyResp.Data.Reason)
	}

	lock := &InstallLock{
		LicenseKey: verifyResp.Data.LicenseKey,
		Company:    verifyResp.Data.Company,
		IssuedAt:   time.Now(),
		ExpiresAt:  verifyResp.Data.ExpiresAt,
		Trial:      verifyResp.Data.Trial,
		MaxUsers:   0,          // 私域部署:不限制用户数
		Features:   []string{}, // 私域部署:不限制功能
		Version:    lc.cfg.CurrentVersion,
	}
	return lock, nil
}

// CheckOTAUpdate 检查 OTA 升级（调用平台 /api/platform/ota/check）
func (lc *LicenseChecker) CheckOTAUpdate() (*otaPackage, error) {
	lc.mu.RLock()
	lock := lc.installLock
	lc.mu.RUnlock()

	if lock == nil {
		return nil, ErrLicenseMissing
	}

	url := fmt.Sprintf("%s/api/platform/ota/check?current_version=%s", lc.cfg.ServerURL, lc.cfg.CurrentVersion)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := lc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OTA 检查请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查 HTTP 状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OTA 检查 HTTP 异常: status=%d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 OTA 响应失败: %w", err)
	}

	var result struct {
		Code int         `json:"code"`
		Msg  string      `json:"msg"`
		Data *otaPackage `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析 OTA 响应失败: %w (body=%s)", err, string(respBody))
	}

	if result.Code != 200 {
		return nil, fmt.Errorf("OTA 检查失败: code=%d, msg=%s", result.Code, result.Msg)
	}

	// 如果 data 为 nil 或者版本相同，返回 nil 表示无升级
	if result.Data == nil || result.Data.Version == lc.cfg.CurrentVersion {
		return nil, nil
	}

	return result.Data, nil
}

// ============== 平台 API 调用 ==============

// verifyLicenseResponse 平台验证响应
type verifyLicenseResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Valid         bool      `json:"valid"`
		LicenseKey    string    `json:"license_key"`
		Company       string    `json:"company"`
		Status        string    `json:"status"`
		ExpiresAt     time.Time `json:"expires_at"`
		RemainingDays int       `json:"remaining_days"`
		MaxUsers      int       `json:"max_users"`
		Features      []string  `json:"features"`
		Trial         bool      `json:"trial"`
		Reason        string    `json:"reason"`
	} `json:"data"`
}

// callPlatformVerify 调用平台 /api/platform/auth/verify
func (lc *LicenseChecker) callPlatformVerify(licenseKey string) (*verifyLicenseResponse, error) {
	url := lc.cfg.ServerURL + "/api/platform/auth/verify"

	body, _ := json.Marshal(map[string]string{
		"license_key": licenseKey,
	})

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := lc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求平台失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查 HTTP 状态码（平台正常返回 200，非 200 视为服务异常）
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("平台 HTTP 异常: status=%d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var result verifyLicenseResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w (body=%s)", err, string(respBody))
	}

	if result.Code != 200 {
		return nil, fmt.Errorf("平台返回错误: code=%d, msg=%s", result.Code, result.Msg)
	}

	return &result, nil
}

// heartbeatRequest 心跳请求
type heartbeatRequest struct {
	InstallID  string          `json:"install_id"`
	LicenseKey string          `json:"license_key"`
	Version    string          `json:"version"`
	HostInfo   json.RawMessage `json:"host_info"`
	Metrics    json.RawMessage `json:"metrics"`
	Timestamp  time.Time       `json:"timestamp"`
}

// heartbeatResponse 心跳响应
// 对应平台端 service.HeartbeatResponse
type heartbeatResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		LicenseOK  bool         `json:"license_ok"`
		License    *licenseInfo `json:"license,omitempty"`
		OTA        *otaPackage  `json:"ota,omitempty"`
		ServerTime time.Time    `json:"server_time"`
		NoticeURL  string       `json:"notice_url"`
	} `json:"data"`
}

// licenseInfo 心跳响应中携带的授权信息（对应平台 LicenseInfoDTO）
type licenseInfo struct {
	LicenseKey    string    `json:"license_key"`
	Company       string    `json:"company"`
	Status        string    `json:"status"`
	ExpiresAt     time.Time `json:"expires_at"`
	RemainingDays int       `json:"remaining_days"`
	MaxUsers      int       `json:"max_users"`
	Features      []string  `json:"features"`
	Trial         bool      `json:"trial"`
	IsValid       bool      `json:"is_valid"`
}

// otaPackage OTA 升级包
type otaPackage struct {
	Version         string    `json:"version"`
	Strategy        string    `json:"strategy"`
	MinVersion      string    `json:"min_version"`
	DownloadURL     string    `json:"download_url"`
	Checksum        string    `json:"checksum"`
	Size            int64     `json:"size"`
	ReleaseNotes    string    `json:"release_notes"`
	Features        []string  `json:"features"`
	BugFixes        []string  `json:"bug_fixes"`
	BreakingChanges []string  `json:"breaking_changes"`
	ReleasedAt      time.Time `json:"released_at"`
}

// SendHeartbeat 上报心跳
// 对应 LICENSE_OTA_DESIGN.md 第 5.3 节
func (lc *LicenseChecker) SendHeartbeat() error {
	lc.mu.RLock()
	lock := lc.installLock
	lc.mu.RUnlock()

	if lock == nil {
		return ErrLicenseMissing
	}

	// 构造心跳请求
	hostInfo, _ := json.Marshal(map[string]string{
		"hostname": getHostname(),
		"platform": getOSInfo(),
	})
	metrics, _ := json.Marshal(map[string]any{
		"timestamp": time.Now().Unix(),
	})

	req := heartbeatRequest{
		InstallID:  lock.InstallID,
		LicenseKey: lock.LicenseKey,
		Version:    lc.cfg.CurrentVersion,
		HostInfo:   hostInfo,
		Metrics:    metrics,
		Timestamp:  time.Now(),
	}

	url := lc.cfg.ServerURL + "/api/platform/heartbeat"
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := lc.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("心跳上报失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查 HTTP 状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("心跳 HTTP 异常: status=%d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取心跳响应失败: %w", err)
	}

	var result heartbeatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("解析心跳响应失败: %w (body=%s)", err, string(respBody))
	}

	if result.Code != 200 {
		return fmt.Errorf("心跳上报失败: code=%d, msg=%s", result.Code, result.Msg)
	}

	// 同步平台下发的授权信息（如过期时间被提前、状态变更等）
	if result.Data.License != nil {
		lc.mu.Lock()
		if lc.installLock != nil {
			// 平台返回的过期时间更严格时同步到本地
			if result.Data.License.ExpiresAt.Before(lc.installLock.ExpiresAt) {
				lc.installLock.ExpiresAt = result.Data.License.ExpiresAt
				_ = lc.writeInstallLockLocked()
			}
			// 授权被平台暂停/吊销/过期时同步状态
			if !result.Data.License.IsValid {
				switch result.Data.License.Status {
				case "suspended":
					lc.status.Store(LicenseStatusSuspended)
				case "revoked":
					lc.status.Store(LicenseStatusRevoked)
				case "expired":
					lc.status.Store(LicenseStatusExpired)
				}
			}
		}
		lc.mu.Unlock()
	}

	// 公告 URL
	if result.Data.NoticeURL != "" {
		logger.Info(fmt.Sprintf("平台公告: %s", result.Data.NoticeURL))
	}

	// 检查 OTA 推送
	if result.Data.OTA != nil && result.Data.OTA.Version != lc.cfg.CurrentVersion {
		logger.Info(fmt.Sprintf("发现新版本: %s (策略: %s)", result.Data.OTA.Version, result.Data.OTA.Strategy))
		// 根据策略处理
		switch result.Data.OTA.Strategy {
		case "force", "security":
			logger.Warn(fmt.Sprintf("收到强制升级通知: %s", result.Data.OTA.Version))
		case "recommended":
			logger.Info(fmt.Sprintf("收到推荐升级通知: %s", result.Data.OTA.Version))
		}
	}

	return nil
}

// ============== 工具函数 ==============

func getHostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

func getOSInfo() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}

// InitState 初始化状态机枚举
const (
	InitStateNotInstalled     = "NOT_INSTALLED"     // 无 install.lock
	InitStateHasLicense       = "HAS_LICENSE"       // 有 license 无 admin
	InitStateHasAdmin         = "HAS_ADMIN"         // 有 admin 未首次改密
	InitStateInitialized      = "INITIALIZED"       // 全部就绪
	InitStateLicenseExpired   = "LICENSE_EXPIRED"   // 授权过期
	InitStateLicenseSuspended = "LICENSE_SUSPENDED" // 授权暂停
	InitStateLicenseRevoked   = "LICENSE_REVOKED"   // 授权吊销
)

// InitStatus 初始化状态对象（公开 API 用）
type InitStatus struct {
	State              string          `json:"state"`
	Initialized        bool            `json:"initialized"`
	HasLicense         bool            `json:"has_license"`
	HasAdmin           bool            `json:"has_admin"`
	AdminUsername      string          `json:"admin_username,omitempty"`
	MustChangePassword bool            `json:"must_change_password"`
	License            *LicenseSummary `json:"license,omitempty"`
	Version            string          `json:"version"`
	InstallID          string          `json:"install_id"`
}

// LicenseSummary 授权摘要（公开）
type LicenseSummary struct {
	LicenseKey    string    `json:"license_key"`
	Company       string    `json:"company"`
	ContactEmail  string    `json:"contact_email,omitempty"`
	ExpiresAt     time.Time `json:"expires_at"`
	RemainingDays int       `json:"remaining_days"`
	Trial         bool      `json:"trial"`
	MaxUsers      int       `json:"max_users"`
	Features      []string  `json:"features"`
	Status        string    `json:"status"`
}

// GetInitStatus 获取初始化状态（公开 API 用）
// 状态机：NOT_INSTALLED → HAS_LICENSE → HAS_ADMIN → INITIALIZED
// 叠加状态：LICENSE_EXPIRED / LICENSE_SUSPENDED / LICENSE_REVOKED
func (lc *LicenseChecker) GetInitStatus() *InitStatus {
	lc.mu.RLock()
	lock := lc.installLock
	lc.mu.RUnlock()

	status := &InitStatus{
		Version:   lc.cfg.CurrentVersion,
		State:     InitStateNotInstalled,
		InstallID: "",
	}

	if lock == nil {
		// 无 install.lock
		status.State = InitStateNotInstalled
		status.Initialized = false
		status.HasLicense = false
		status.HasAdmin = false
		return status
	}

	// 基础字段
	status.InstallID = lock.InstallID
	status.HasLicense = lock.HasLicense()
	status.HasAdmin = lock.HasAdminRecord()
	status.AdminUsername = lock.AdminUsername
	status.MustChangePassword = lock.MustChangePassword()
	status.Initialized = lock.IsInitialized()

	// License 摘要
	if lock.HasLicense() {
		licSummary := &LicenseSummary{
			LicenseKey:    lock.LicenseKey,
			Company:       lock.Company,
			ContactEmail:  lock.ContactEmail,
			ExpiresAt:     lock.ExpiresAt,
			RemainingDays: lock.RemainingDays(),
			Trial:         lock.Trial,
			MaxUsers:      lock.MaxUsers,
			Features:      lock.Features,
			Status:        string(licenseStatusForState(lc)),
		}
		status.License = licSummary
	}

	// 开源版状态机判定（不再包含任何授权状态）：
	//  NOT_INSTALLED -> HAS_ADMIN -> INITIALIZED
	switch {
	case status.Initialized:
		status.State = InitStateInitialized
	case status.HasAdmin:
		status.State = InitStateHasAdmin
	default:
		status.State = InitStateNotInstalled
	}

	return status
}

// licenseStatusForState 取当前 LicenseChecker 状态字符串
func licenseStatusForState(lc *LicenseChecker) LicenseStatus {
	return lc.Status()
}

// BindLicenseInit 初始化绑定（Step 2）
// 与 BindLicense 区别：不要求必须已绑定，可首次绑定
func (lc *LicenseChecker) BindLicenseInit(licenseKey, contactEmail, contactPhone string) (*InstallLock, error) {
	verifyResp, err := lc.callPlatformVerify(licenseKey)
	if err != nil {
		return nil, fmt.Errorf("平台验证失败: %w", err)
	}
	if !verifyResp.Data.Valid {
		return nil, fmt.Errorf("授权码无效: %s", verifyResp.Data.Reason)
	}

	lc.mu.Lock()
	existing := lc.installLock
	installID := "inst-" + uuid.New().String()
	if existing != nil && existing.InstallID != "" {
		installID = existing.InstallID
	}
	adminUsername := ""
	adminInitialized := false
	var initializedAt *time.Time
	if existing != nil {
		adminUsername = existing.AdminUsername
		adminInitialized = existing.AdminInitialized
		initializedAt = existing.InitializedAt
	}
	lc.mu.Unlock()

	now := time.Now()
	lock := &InstallLock{
		LicenseKey:       verifyResp.Data.LicenseKey,
		InstallID:        installID,
		Company:          verifyResp.Data.Company,
		ContactEmail:     contactEmail,
		ContactPhone:     contactPhone,
		IssuedAt:         now,
		ExpiresAt:        verifyResp.Data.ExpiresAt,
		Trial:            verifyResp.Data.Trial,
		MaxUsers:         0,          // 私域部署:不限制用户数
		Features:         []string{}, // 私域部署:不限制功能
		Version:          lc.cfg.CurrentVersion,
		AdminUsername:    adminUsername,
		AdminInitialized: adminInitialized,
		InitializedAt:    initializedAt,
	}

	if err := lc.SaveInstallLock(lock); err != nil {
		return nil, err
	}

	lc.status.Store(LicenseStatusActive)
	atomic.StoreInt32(&lc.graceCount, int32(lc.cfg.GraceCount))
	return lock, nil
}

// SetAdminInit 设置超管记录（Step 3）
// 在 system_users 表创建超管成功后调用，install.lock 记录用户名
func (lc *LicenseChecker) SetAdminInit(adminUsername string) error {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if lc.installLock == nil {
		// 开源版：install.lock 缺失时直接创建（无需 LicenseKey）。
		// 不再要求首次改密，管理员创建即视为初始化完成。
		now := time.Now()
		lc.installLock = &InstallLock{
			InstallID:        "inst-" + uuid.New().String(),
			AdminUsername:    adminUsername,
			AdminInitialized: true, // 开源版：不再强制首次改密
			InitializedAt:    &now,
			Version:          lc.cfg.CurrentVersion,
		}
		return lc.writeInstallLockLocked()
	}

	if lc.installLock.AdminUsername == adminUsername {
		// 已设置过，幂等
		return nil
	}

	lc.installLock.AdminUsername = adminUsername
	// 开源版：不再要求首次改密，管理员创建即视为初始化完成。
	now := time.Now()
	lc.installLock.AdminInitialized = true
	lc.installLock.InitializedAt = &now
	return lc.writeInstallLockLocked()
}

// MarkAdminInitialized 标记超管已首次改密（Step 5）
func (lc *LicenseChecker) MarkAdminInitialized() error {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if lc.installLock == nil {
		return ErrLicenseMissing
	}

	if lc.installLock.AdminInitialized {
		return nil // 幂等
	}

	now := time.Now()
	lc.installLock.AdminInitialized = true
	lc.installLock.InitializedAt = &now
	return lc.writeInstallLockLocked()
}

// HasAnySystemUser 用于 SystemInitService 检查（公开 API）
// 注：实际查询走 repository，这里返回的是 install.lock 的判定
// 业务侧校验"system_users 是否有任何用户"应走 SystemInitService
func (lc *LicenseChecker) HasInstallLockAdmin() bool {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	return lc.installLock != nil && lc.installLock.AdminUsername != ""
}

// IsInitialized install.lock 是否完全初始化
// 判定：License 已绑 + 超管已记录 + 超管已首次改密
func (lc *LicenseChecker) IsInitialized() bool {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	return lc.installLock != nil && lc.installLock.IsInitialized()
}

// GetAdminUsername 返回 install.lock 中记录的超管用户名
func (lc *LicenseChecker) GetAdminUsername() string {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	if lc.installLock == nil {
		return ""
	}
	return lc.installLock.AdminUsername
}
