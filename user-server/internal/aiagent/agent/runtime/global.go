package agent_runtime

import (
	"sync"
)

var (
	globalRuntime     AgentRuntime
	globalRuntimeOnce sync.Once
	globalRuntimeMu   sync.RWMutex
)

// InitGlobalRuntime 注入全局运行时（可重复覆盖，便于测试与热重载）。
// 与 llm.InitGlobalDispatcher 对称：启动期由 wiring 层调用。
func InitGlobalRuntime(rt AgentRuntime) {
	globalRuntimeMu.Lock()
	defer globalRuntimeMu.Unlock()
	globalRuntime = rt
	globalRuntimeOnce.Do(func() {})
}

// GetGlobalRuntime 获取全局运行时；未初始化时返回 nil。
func GetGlobalRuntime() AgentRuntime {
	globalRuntimeMu.RLock()
	defer globalRuntimeMu.RUnlock()
	return globalRuntime
}

// MustGetGlobalRuntime 获取全局运行时；未初始化时 panic（仅限启动后确定已装配的路径）。
func MustGetGlobalRuntime() AgentRuntime {
	rt := GetGlobalRuntime()
	if rt == nil {
		panic("agent_runtime: global runtime not initialized; call InitGlobalRuntime first")
	}
	return rt
}
