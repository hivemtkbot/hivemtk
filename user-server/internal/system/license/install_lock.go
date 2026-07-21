package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// InstallLock install.lock 文件结构
// 角色：部署凭证(持久化、有法律效力)
//
// 私域独立部署约定(2026-07-21 架构决策):
//   - 仅按 ExpiresAt 控制授权有效期
//   - 不限制功能(Features 保留为兼容字段,始终为空数组)
//   - 不限制用户数(MaxUsers 保留为兼容字段,始终为 0)
type InstallLock struct {
	LicenseKey   string    `json:"license_key"`
	InstallID    string    `json:"install_id"`
	Company      string    `json:"company"`
	ContactEmail string    `json:"contact_email"`
	ContactPhone string    `json:"contact_phone"`
	IssuedAt     time.Time `json:"issued_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	Trial        bool      `json:"trial"`
	MaxUsers     int       `json:"max_users"` // 兼容旧字段,私域部署固定 0
	Features     []string  `json:"features"`  // 兼容旧字段,私域部署固定空数组
	Version      string    `json:"version"`

	AdminUsername    string     `json:"admin_username,omitempty"`
	AdminInitialized bool       `json:"admin_initialized"`
	InitializedAt    *time.Time `json:"initialized_at,omitempty"`

	// 平台签名（HMAC-SHA256(payload, PlatformSecret)），用于本地防篡改校验
	Signature string `json:"signature,omitempty"`
}

// canonicalPayload 生成参与签名的规范化负载（不含 Signature 字段自身）
func (l *InstallLock) canonicalPayload() ([]byte, error) {
	// 拷贝结构体并清空签名
	clone := *l
	clone.Signature = ""
	return json.Marshal(&clone)
}

// Sign 使用平台密钥对 install.lock 内容签名
func (l *InstallLock) Sign(platformSecret string) error {
	if platformSecret == "" {
		return errors.New("platform secret 不能为空")
	}
	payload, err := l.canonicalPayload()
	if err != nil {
		return err
	}
	mac := hmac.New(sha256.New, []byte(platformSecret))
	mac.Write(payload)
	l.Signature = hex.EncodeToString(mac.Sum(nil))
	return nil
}

// VerifySignature 验证 install.lock 签名
// 平台签发时附带 Signature 字段；本地存储时也必须带签名
func (l *InstallLock) VerifySignature(platformSecret string) error {
	if platformSecret == "" {
		return errors.New("platform secret 未配置，无法校验签名")
	}
	if l.Signature == "" {
		return errors.New("install.lock 缺少签名")
	}
	payload, err := l.canonicalPayload()
	if err != nil {
		return err
	}
	mac := hmac.New(sha256.New, []byte(platformSecret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(strings.ToLower(l.Signature)), []byte(expected)) {
		return errors.New("install.lock 签名不匹配")
	}
	return nil
}

// Marshal 序列化为 JSON
func (l *InstallLock) Marshal() ([]byte, error) {
	return json.MarshalIndent(l, "", "  ")
}

// UnmarshalInstallLock 从 JSON 解析
func UnmarshalInstallLock(data []byte) (*InstallLock, error) {
	var lock InstallLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, err
	}
	if lock.Features == nil {
		lock.Features = []string{}
	}
	return &lock, nil
}

// IsExpired 检查本地授权是否已过期
//
// 开源版说明：hivemtk 现已全面开源，install.lock 不再承载 LicenseKey，
// 因此无 LicenseKey 时视为永久有效（永不过期）。仅当历史遗留 install.lock
// 仍带 LicenseKey + 签名时才走原过期判定逻辑（兼容旧数据平滑迁移）。
func (l *InstallLock) IsExpired() bool {
	if l == nil {
		return true
	}
	// 开源版：无 LicenseKey 即永久有效（不再做任何过期判定）
	if l.LicenseKey == "" {
		return false
	}
	// 1. 基础时间检查
	if time.Now().After(l.ExpiresAt) {
		return true
	}
	// 2. 时间合理性检查：拒绝超过 50 年的过期时间（防止篡改到无限期）
	if l.ExpiresAt.After(time.Now().Add(50 * 365 * 24 * time.Hour)) {
		return true // 视为异常篡改
	}
	// 3. 签名缺失视为无效（除非处于安装引导阶段）
	if l.Signature == "" {
		// 安装引导阶段允许无签名（首次安装时还未签）
		if !l.HasLicense() {
			return false
		}
		return true
	}
	return false
}

// RemainingDays 剩余天数
func (l *InstallLock) RemainingDays() int {
	if l == nil || l.ExpiresAt.Before(time.Now()) {
		return 0
	}
	return int(l.ExpiresAt.Sub(time.Now()).Hours() / 24)
}

// IsInitialized 系统是否已完成初始化
// 开源版：不再要求 LicenseKey，只要超管已创建并完成初始化即视为完成。
func (l *InstallLock) IsInitialized() bool {
	return l != nil &&
		l.AdminUsername != "" &&
		l.AdminInitialized &&
		l.InitializedAt != nil
}

// HasLicense 是否已绑定授权码（NOT_INSTALLED → HAS_LICENSE）
func (l *InstallLock) HasLicense() bool {
	return l != nil && l.LicenseKey != ""
}

// HasAdminRecord install.lock 中是否记录了超管（HAS_LICENSE → HAS_ADMIN）
func (l *InstallLock) HasAdminRecord() bool {
	return l != nil && l.AdminUsername != ""
}

// MustChangePassword 是否强制改密（install.lock 层面）
func (l *InstallLock) MustChangePassword() bool {
	return l != nil &&
		l.HasAdminRecord() &&
		!l.AdminInitialized
}

// MarkAdminInitializedStandalone 独立标记 install.lock 为 AdminInitialized
// 适用场景：LicenseChecker 未注册（单测环境 / 早期启动）
// 工作原理：
//  1. 读取 install.lock
//  2. 若 AdminUsername 为空 → 返回错误（必须先有超管）
//  3. 设置 AdminInitialized=true、InitializedAt=now
//  4. 写回 install.lock（保留签名）
//
// 注：签名需要 PlatformSecret，但该函数调用方可能没配置 secret，因此做"无签名也可写"的容错。
//
//	签名在下次 BindLicense 时会被刷新。
func MarkAdminInitializedStandalone() error {
	path := getInstallLockPathStandalone()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("install.lock 不存在: %s", path)
		}
		return fmt.Errorf("读取 install.lock 失败: %w", err)
	}
	lock, err := UnmarshalInstallLock(data)
	if err != nil {
		return fmt.Errorf("解析 install.lock 失败: %w", err)
	}
	if lock.AdminUsername == "" {
		return errors.New("install.lock 中尚未记录超管，无法标记为已初始化")
	}
	if lock.AdminInitialized {
		return nil // 幂等
	}
	now := time.Now()
	lock.AdminInitialized = true
	lock.InitializedAt = &now

	// 与 LicenseChecker.writeInstallLockLocked 保持一致：用 PLATFORM_LICENSE_SECRET 补签名，
	// 否则写回的 install.lock 无签名，下次加载会被 IsExpired 误判为失效。
	if secret := os.Getenv("PLATFORM_LICENSE_SECRET"); secret != "" {
		_ = lock.Sign(secret)
	}

	out, err := lock.Marshal()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, out, 0600); err != nil {
		return fmt.Errorf("写 install.lock 失败: %w", err)
	}
	return nil
}

// getInstallLockPathStandalone 独立读取 install.lock 路径（不走 LicenseChecker 配置）
// 优先级：环境变量 INSTALL_LOCK_PATH > /opt/mtk/install.lock > ./install.lock
func getInstallLockPathStandalone() string {
	if p := os.Getenv("INSTALL_LOCK_PATH"); p != "" {
		return p
	}
	if _, err := os.Stat("/opt/mtk/install.lock"); err == nil {
		return "/opt/mtk/install.lock"
	}
	return "install.lock"
}

// LoadInstallLockPublic 公开加载 install.lock（供 service 层忘记密码流程使用）
// 返回 nil + nil 表示文件不存在
// 返回 nil + error 表示读取/解析错误
func LoadInstallLockPublic() (*InstallLock, error) {
	path := getInstallLockPathStandalone()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取 install.lock 失败: %w", err)
	}
	lock, err := UnmarshalInstallLock(data)
	if err != nil {
		return nil, fmt.Errorf("解析 install.lock 失败: %w", err)
	}
	return lock, nil
}
