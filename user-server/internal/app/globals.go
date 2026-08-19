package app

import (
	"sync"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/service"
)


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


var globalBridgeIngressSvc *service.InboxIngressService

// SetBridgeIngressSvc 由 router.Setup 在构造 InboxIngressService 后注入
//
// 同步设置 service 包的全局引用（service.DeliverBridgeOutbound 走 service→service 直接调用，
// 不再经 bridge 包 callback 间接层；2026-08-18 修复主动外联走死通道 bug）。
func SetBridgeIngressSvc(s *service.InboxIngressService) {
	globalBridgeIngressSvc = s
	service.SetGlobalInboxIngressService(s)
}

// GetBridgeIngressSvc 读取桥接入站服务（装配前为 nil）
func GetBridgeIngressSvc() *service.InboxIngressService { return globalBridgeIngressSvc }

