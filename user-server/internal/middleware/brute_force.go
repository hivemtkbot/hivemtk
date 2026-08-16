package middleware

import (
	"net/http"
	"sync"
	"time"

	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// BruteForceGuard 防爆破中间件
// 用于保护敏感接口（如授权码绑定、密码修改等）防暴力破解
// 设计要点：
//  1. 基于 IP + 端点 key 计数
//  2. 滑动窗口（最近 15 分钟内）
//  3. 超过阈值后锁定一段时间（递增）
//
// 4. 扩展：超过阈值后写入安全告警表（由调用方或 service 触发）
type bruteForceEntry struct {
	failures    int
	firstAt     time.Time
	lockedUntil time.Time
}

type bruteForceProtector struct {
	mu      sync.RWMutex
	entries map[string]*bruteForceEntry
}

// globalBruteForce 全局防爆破注册表
var globalBruteForce = &bruteForceProtector{
	entries: make(map[string]*bruteForceEntry),
}

// BruteForceLockCallback 锁定时触发的回调
// 用于在锁定时把告警写入数据库 / 推送通知
// endpoint: 触发锁定的端点（如 "auth.login"）
// retryAfter: 剩余锁定秒数
type BruteForceLockCallback func(c *gin.Context, endpoint string, retryAfter int)

// globalBruteForceOnLock 全局锁定回调（可选，由 service 层注册）
var globalBruteForceOnLock BruteForceLockCallback

// SetBruteForceLockCallback 注册锁定回调
// 由 service 层调用（避免 middleware → service 循环依赖）
func SetBruteForceLockCallback(cb BruteForceLockCallback) {
	globalBruteForceOnLock = cb
}

// BruteForceConfig 防爆破配置
type BruteForceConfig struct {
	Endpoint string
	Window time.Duration
	MaxFailures int
	LockDuration time.Duration
}

// DefaultBruteForceConfig 默认防爆破配置
var DefaultBruteForceConfig = BruteForceConfig{
	Window:       15 * time.Minute,
	MaxFailures:  5,
	LockDuration: 30 * time.Minute,
}

// BruteForceGuard 防爆破中间件
// 用法：
//
//	auth.POST("/license/bind", middleware.BruteForceGuard("license.bind"), controller.BindLicense)
//	控制器中失败时调用：middleware.RecordBruteForceFailure(c, "license.bind")
//	控制器中成功时调用：middleware.ClearBruteForceFailure(c, "license.bind")
func BruteForceGuard(endpoint string) gin.HandlerFunc {
	cfg := DefaultBruteForceConfig
	cfg.Endpoint = endpoint

	return func(c *gin.Context) {
		clientKey := c.ClientIP() + "|" + endpoint
		entry := getBruteForceEntry(clientKey)

		now := time.Now()

		// v3 审计 P1-22 修复：使用单一写锁覆盖整个判定+记录流程
		// 原：RLock 读 + RUnlock 之后才 RecordFailure → TOCTOU 窗口
		// 新：全程持有 Lock 串行化，同一 clientKey 不会并行判定
		globalBruteForce.mu.Lock()
		locked := !entry.lockedUntil.IsZero() && now.Before(entry.lockedUntil)
		if !locked {
			// 在锁内做完整"检查 + 自增"原子序列
			entry.failures++
			if entry.failures >= DefaultBruteForceConfig.MaxFailures {
				entry.lockedUntil = now.Add(DefaultBruteForceConfig.LockDuration)
				entry.failures = 0
			}
		}
		lockedUntil := entry.lockedUntil
		failures := entry.failures
		globalBruteForce.mu.Unlock()

		if locked {
			retryAfter := int(time.Until(lockedUntil).Seconds())
			if retryAfter < 1 {
				retryAfter = 1
			}
			c.Header("Retry-After", itoa(retryAfter))
			if globalBruteForceOnLock != nil {
				globalBruteForceOnLock(c, endpoint, retryAfter)
			}
			response.Error(c, http.StatusTooManyRequests, "尝试次数过多，请稍后再试")
			c.Abort()
			return
		}
		_ = failures // 计数已自增，仅用于未来 metric

		c.Next()
	}
}

// RecordBruteForceFailure 记录一次失败
// 在 controller 中检测到失败时调用
func RecordBruteForceFailure(c *gin.Context, endpoint string) {
	cfg := DefaultBruteForceConfig
	clientKey := c.ClientIP() + "|" + endpoint
	now := time.Now()

	globalBruteForce.mu.Lock()
	defer globalBruteForce.mu.Unlock()

	entry, exists := globalBruteForce.entries[clientKey]
	if !exists {
		entry = &bruteForceEntry{firstAt: now}
		globalBruteForce.entries[clientKey] = entry
	}

	if now.Sub(entry.firstAt) > cfg.Window {
		entry.failures = 0
		entry.firstAt = now
		entry.lockedUntil = time.Time{}
	}

	entry.failures++

	if entry.failures >= cfg.MaxFailures {
		multiplier := 1
		switch {
		case entry.failures >= cfg.MaxFailures*8:
			multiplier = 8
		case entry.failures >= cfg.MaxFailures*4:
			multiplier = 4
		case entry.failures >= cfg.MaxFailures*2:
			multiplier = 2
		}
		entry.lockedUntil = now.Add(time.Duration(multiplier) * cfg.LockDuration)
	}
}

// ClearBruteForceFailure 清除失败计数（成功时调用）
func ClearBruteForceFailure(c *gin.Context, endpoint string) {
	clientKey := c.ClientIP() + "|" + endpoint
	globalBruteForce.mu.Lock()
	defer globalBruteForce.mu.Unlock()
	delete(globalBruteForce.entries, clientKey)
}

// IsBruteForceLocked 检查指定 IP+端点是否处于锁定状态
// 用于 service 层在写入 login_events 时附加风险信息
func IsBruteForceLocked(ip, endpoint string) (bool, time.Duration) {
	clientKey := ip + "|" + endpoint
	globalBruteForce.mu.RLock()
	defer globalBruteForce.mu.RUnlock()
	entry, exists := globalBruteForce.entries[clientKey]
	if !exists {
		return false, 0
	}
	now := time.Now()
	if entry.lockedUntil.IsZero() || !now.Before(entry.lockedUntil) {
		return false, 0
	}
	return true, time.Until(entry.lockedUntil)
}

// GetBruteForceFailureCount 获取当前失败次数（用于风险评估）
func GetBruteForceFailureCount(ip, endpoint string) int {
	clientKey := ip + "|" + endpoint
	globalBruteForce.mu.RLock()
	defer globalBruteForce.mu.RUnlock()
	entry, exists := globalBruteForce.entries[clientKey]
	if !exists {
		return 0
	}
	return entry.failures
}

// ResetBruteForceForTest 重置所有防爆破状态（仅用于测试）
func ResetBruteForceForTest() {
	globalBruteForce.mu.Lock()
	defer globalBruteForce.mu.Unlock()
	globalBruteForce.entries = make(map[string]*bruteForceEntry)
}

func getBruteForceEntry(clientKey string) *bruteForceEntry {
	globalBruteForce.mu.RLock()
	defer globalBruteForce.mu.RUnlock()
	if entry, exists := globalBruteForce.entries[clientKey]; exists {
		return entry
	}
	return &bruteForceEntry{}
}

// itoa 避免引入 strconv 依赖
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	negative := false
	if i < 0 {
		negative = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

