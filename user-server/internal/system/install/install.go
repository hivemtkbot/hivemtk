// Package install 提供 install.lock 文件读写（开源版精简实现）
//
// 背景：hivemtk 全面开源后不再有 License / 授权流程。
// install.lock 仅用于记录"本机安装"的最小状态：
//   - install_id     一次安装 = 一个唯一标识（UUID）
//   - install_time   安装时间（UTC RFC3339）
//   - admin_username 超级管理员账号（创建后写入）
//   - initialized    是否已创建超管（"true"/"false"）
//   - version        客户端版本号（用于统计）
//
// 不再包含：license_key / expire_at / company_name / contact_email 等授权相关字段。
package install

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Lock install.lock 文件结构（精简版）
type Lock struct {
	InstallID     string `json:"install_id"`
	InstallTime   string `json:"install_time"`
	AdminUsername string `json:"admin_username"`
	Initialized   bool   `json:"initialized"`
	Version       string `json:"version"`
}

// DefaultInstallLockPath 默认 install.lock 路径
const DefaultInstallLockPath = "./install.lock"

// GetInstallLockPath 解析实际 install.lock 路径
//
// 优先级：
//  1. 环境变量 INSTALL_LOCK_PATH
//  2. ./install.lock
func GetInstallLockPath() string {
	if p := os.Getenv("INSTALL_LOCK_PATH"); p != "" {
		return p
	}
	return DefaultInstallLockPath
}

var (
	mu      sync.RWMutex
	memoLR  *Lock
	memoExp time.Time
)

const memoTTL = 2 * time.Second

// Load 读取 install.lock（带 2 秒内存缓存，文件 IO 在 InitGuard 高频路径上避免抖动）
func Load() (*Lock, error) {
	mu.RLock()
	if memoLR != nil && time.Now().Before(memoExp) {
		lr := *memoLR
		mu.RUnlock()
		return &lr, nil
	}
	mu.RUnlock()

	path := GetInstallLockPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var lr Lock
	if err := json.Unmarshal(data, &lr); err != nil {
		return nil, err
	}
	mu.Lock()
	memoLR = &lr
	memoExp = time.Now().Add(memoTTL)
	mu.Unlock()
	out := lr
	return &out, nil
}

// Save 写回 install.lock（同时刷新内存缓存）
func Save(lr *Lock) error {
	if lr == nil {
		return errors.New("install lock is nil")
	}
	if lr.InstallID == "" {
		lr.InstallID = newInstallID()
	}
	if lr.InstallTime == "" {
		lr.InstallTime = time.Now().UTC().Format(time.RFC3339)
	}
	path := GetInstallLockPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(lr, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	mu.Lock()
	memoLR = lr
	memoExp = time.Now().Add(memoTTL)
	mu.Unlock()
	return nil
}

// MarkAdminInitialized 标记"超管已创建"（开源版：创建超管即视为初始化完成）
func MarkAdminInitialized(username string) error {
	lr, err := Load()
	if err != nil {
		return err
	}
	if lr == nil {
		lr = &Lock{
			InstallID:   newInstallID(),
			InstallTime: time.Now().UTC().Format(time.RFC3339),
		}
	}
	lr.AdminUsername = strings.TrimSpace(username)
	lr.Initialized = true
	return Save(lr)
}

// MarkAdminInitializedStandalone 兼容旧调用方：标记 install.lock 已初始化
//
// 等价于 MarkAdminInitialized(currentAdminUsername)，
// 仅在 install.lock 已存在且 AdminUsername 非空时使用；
// 否则用空字符串调用，由 install.lock 自动写入新 AdminUsername。
func MarkAdminInitializedStandalone() error {
	lr, err := Load()
	if err != nil {
		return err
	}
	username := ""
	if lr != nil {
		username = lr.AdminUsername
	}
	return MarkAdminInitialized(username)
}

// LoadInstallLockPublic 公开加载 install.lock（不依赖中间件全局）
func LoadInstallLockPublic() (*Lock, error) {
	return Load()
}

// EnsureInstallID 若 install.lock 不存在则创建一份只含 install_id 的文件
func EnsureInstallID(version string) (*Lock, error) {
	lr, err := Load()
	if err != nil {
		return nil, err
	}
	if lr == nil {
		lr = &Lock{
			InstallID:   newInstallID(),
			InstallTime: time.Now().UTC().Format(time.RFC3339),
			Version:     version,
		}
		if err := Save(lr); err != nil {
			return nil, err
		}
		return lr, nil
	}
	// 已存在：若 version 缺失则补一下
	if lr.Version == "" && version != "" {
		lr.Version = version
		_ = Save(lr)
	}
	return lr, nil
}

// Status 初始化状态 DTO（与前端约定保持兼容）
type Status struct {
	State       string `json:"state"`
	Initialized bool   `json:"initialized"`
	HasAdmin    bool   `json:"has_admin"`
	InstallID   string `json:"install_id,omitempty"`
	Version     string `json:"version,omitempty"`
}

// GetStatus 返回当前初始化状态
func GetStatus() *Status {
	lr, err := Load()
	if err != nil || lr == nil {
		return &Status{
			State:       "NOT_INSTALLED",
			Initialized: false,
			HasAdmin:    false,
		}
	}
	st := &Status{
		InstallID: lr.InstallID,
		Version:   lr.Version,
		HasAdmin:  lr.AdminUsername != "",
	}
	if lr.Initialized && lr.AdminUsername != "" {
		st.State = "INITIALIZED"
		st.Initialized = true
	} else if lr.AdminUsername != "" {
		st.State = "HAS_ADMIN"
	} else {
		st.State = "NOT_INSTALLED"
	}
	return st
}

// GetAdminUsername 返回 install.lock 中记录的超管账号
func GetAdminUsername() string {
	lr, err := Load()
	if err != nil || lr == nil {
		return ""
	}
	return lr.AdminUsername
}

func newInstallID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 极小概率失败：用时间戳兜底
		ts := time.Now().UnixNano()
		return "ins-" + hex.EncodeToString([]byte{
			byte(ts >> 56), byte(ts >> 48), byte(ts >> 40), byte(ts >> 32),
			byte(ts >> 24), byte(ts >> 16), byte(ts >> 8), byte(ts),
		})
	}
	return "ins-" + hex.EncodeToString(b[:])
}
