package router

import (
	"sync"

	"marketing/internal/aiagent/llm"
)

// ============================================================================
// 全局依赖持有（2026-07-23）
// ----------------------------------------------------------------------------
// 启动期由 main.go 通过 SetGlobalDispatcher / SetGlobalProviderFailover 注入，
// setupLLMRoutingRoutes / setupLLMProviderRoutes 读取后构造 service/controller。
// 设计取舍：不引入 DI 容器（项目禁用），用最简全局变量 + setter。
// ============================================================================

var (
	globalMu               sync.RWMutex
	globalDispatcher       *llm.Dispatcher
	globalProviderFailover *llm.ProviderFailover
)

// SetGlobalDispatcher 注入全局 dispatcher
func SetGlobalDispatcher(d *llm.Dispatcher) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalDispatcher = d
}

// SetGlobalProviderFailover 注入全局 failover
func SetGlobalProviderFailover(f *llm.ProviderFailover) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalProviderFailover = f
}

// getGlobalDispatcher 读取全局 dispatcher
func getGlobalDispatcher() *llm.Dispatcher {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalDispatcher
}

// getGlobalProviderFailover 读取全局 failover
func getGlobalProviderFailover() *llm.ProviderFailover {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalProviderFailover
}
