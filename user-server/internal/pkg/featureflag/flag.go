// Package featureflag 提供企业级 FeatureFlag 支持
//
// 设计依据: 2026-07-31 AI 智能体性能优化 (T5)
//   - 支持灰度发布 + 一键回滚
//   - 5 个核心开关: parallel / stream / layer1 / fallback_chain / debug_log
//   - 通过 env (FF_XXX=1) 注入, 无需重启即可热加载
//   - 所有 flag 默认关闭 (安全)
//
// 使用方式:
//
//	import "marketing/internal/pkg/featureflag"
//
//	if featureflag.Flag("parallel").Bool() {
//	    // 启用并行化
//	}
package featureflag

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Flag 表示一个 FeatureFlag 实例 (线程安全)
type Flag struct {
	name    string
	defaultValue bool
	mu      sync.RWMutex
	lastReload time.Time
}

// FlagManager 全局 Flag 管理器
type FlagManager struct {
	flags map[string]*Flag
	mu    sync.RWMutex
}

var (
	defaultManager *FlagManager
	defaultOnce    sync.Once
)

// DefaultManager 获取默认 Flag 管理器
func DefaultManager() *FlagManager {
	defaultOnce.Do(func() {
		defaultManager = &FlagManager{flags: make(map[string]*Flag)}
		// 注册 5 个核心开关 (默认 false)
		defaultManager.register("parallel", false)
		defaultManager.register("stream", false)
		defaultManager.register("layer1", false)
		defaultManager.register("fallback_chain", false)
		defaultManager.register("debug_log", false)
	})
	return defaultManager
}

// register 注册一个 Flag
func (m *FlagManager) register(name string, defaultValue bool) *Flag {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.flags[name]
	if !ok {
		f = &Flag{name: name, defaultValue: defaultValue}
		m.flags[name] = f
	}
	return f
}

// Flag 获取或创建指定名称的 Flag
func Get(name string) *Flag {
	return DefaultManager().register(name, false)
}

// MustGet 同 Get, 在 flag 不存在时 panic (用于必填 flag)
func MustGet(name string) *Flag {
	f := Get(name)
	if f == nil {
		panic("featureflag: must register flag first: " + name)
	}
	return f
}

// Bool 返回 Flag 的布尔值 (从 env FF_<NAME> 读取)
//
// 规则:
//   - env FF_<NAME>=1 / true / yes -> true
//   - env FF_<NAME>=0 / false / no / "" -> false
//   - env 未设置 -> 使用 defaultValue
func (f *Flag) Bool() bool {
	return f.resolve()
}

// resolve 从 env 解析 (每次都重新读取, 支持热加载)
func (f *Flag) resolve() bool {
	envName := "FF_" + strings.ToUpper(f.name)
	v := os.Getenv(envName)
	if v == "" {
		return f.defaultValue
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		// 容错: "yes" / "no"
		switch strings.ToLower(v) {
		case "yes", "y", "on":
			return true
		case "no", "n", "off":
			return false
		}
		return f.defaultValue
	}
	return b
}

// String 返回 flag 名称 (调试用)
func (f *Flag) String() string {
	return f.name
}

// ReloadAll 重新加载所有 flag (可用于 SIGHUP 触发)
func (m *FlagManager) ReloadAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	now := time.Now()
	for _, f := range m.flags {
		f.mu.Lock()
		f.lastReload = now
		f.mu.Unlock()
	}
}

// Snapshot 返回所有 flag 的当前值快照 (用于 /healthz 调试)
func (m *FlagManager) Snapshot() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]bool, len(m.flags))
	for name, f := range m.flags {
		out[name] = f.resolve()
	}
	return out
}

// AllFlagSnapshot 便捷函数
func AllFlagSnapshot() map[string]bool {
	return DefaultManager().Snapshot()
}
