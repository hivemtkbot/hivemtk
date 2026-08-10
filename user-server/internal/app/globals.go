package app

import (
	"sync"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/service"
)

// ============================================================================
// 全局依赖持有（P1-1：原 router/globals.go 迁入装配包）
// ----------------------------------------------------------------------------
// 启动期由 main.go 通过 SetGlobalDispatcher / SetGlobalProviderFailover 注入，
// router 层读取后构造 service/controller。
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

// GetGlobalDispatcher 读取全局 dispatcher
func GetGlobalDispatcher() *llm.Dispatcher {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalDispatcher
}

// GetGlobalProviderFailover 读取全局 failover
func GetGlobalProviderFailover() *llm.ProviderFailover {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalProviderFailover
}

// ============================================================================
// 桥接入站服务持有（原 router 包级变量 bridgeIngressSvc，P1-1 随装配收敛迁入）
// ============================================================================

var globalBridgeIngressSvc *service.InboxIngressService

// SetBridgeIngressSvc 由 router.Setup 在构造 InboxIngressService 后注入
func SetBridgeIngressSvc(s *service.InboxIngressService) { globalBridgeIngressSvc = s }

// GetBridgeIngressSvc 读取桥接入站服务（装配前为 nil）
func GetBridgeIngressSvc() *service.InboxIngressService { return globalBridgeIngressSvc }
