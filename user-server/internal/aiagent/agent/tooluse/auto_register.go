package tooluse

import "sync"

// ============================================================================
// ToolProvider 自注册机制（+ 优化）
// ----------------------------------------------------------------------------
// 设计目标：允许第三方包在 init() 函数中自注册 Provider，
// 无需修改 router.registerAllAgentTools 核心装配代码。
//
// 使用方式（第三方包）：
//
//	package mytools
//
//	import "hivemtk-user/internal/aiagent/agent/tooluse"
//
//	type MyProvider struct{}
//
//	func (p *MyProvider) Name() string { return "my" }
//	// ... 实现其他方法
//
//	func init() {
//	    tooluse.RegisterToolProvider(&MyProvider{})
//	}
//
// 然后在 main 包中通过空白导入触发 init：
//
//	import _ "hivemtk-user/internal/mytools"
//
// router.Setup 启动时会调用 GetAutoRegisteredProviders() 获取所有自注册 Provider，
// 并通过 ProviderRegistry.RegisterProvider 装配。
//
// 注意事项：
//   - 自注册的 Provider 默认启用，可在调用方通过 ProviderConfig 关闭
//   - 自注册顺序依赖 Go init 执行顺序（按导入顺序），不建议依赖特定顺序
//   - 测试中可通过 ClearAutoRegisteredProviders 清理状态
// ============================================================================

var (
	autoRegisteredProviders []ToolProvider
	autoRegisterMu          sync.Mutex
)

// RegisterToolProvider 自注册一个 Provider
//
// 线程安全。通常在包 init() 中调用。
// 重复注册同名 Provider 不会报错（后注册的覆盖），但建议调用方避免重复。
func RegisterToolProvider(p ToolProvider) {
	if p == nil {
		return
	}
	autoRegisterMu.Lock()
	defer autoRegisterMu.Unlock()
	// 检查重复（按 Name）
	for i, existing := range autoRegisteredProviders {
		if existing.Name() == p.Name() {
			autoRegisteredProviders[i] = p
			return
		}
	}
	autoRegisteredProviders = append(autoRegisteredProviders, p)
}

// GetAutoRegisteredProviders 返回所有自注册的 Provider（拷贝）
//
// 调用方：router.registerAllAgentTools
func GetAutoRegisteredProviders() []ToolProvider {
	autoRegisterMu.Lock()
	defer autoRegisterMu.Unlock()
	out := make([]ToolProvider, len(autoRegisteredProviders))
	copy(out, autoRegisteredProviders)
	return out
}

// ClearAutoRegisteredProviders 清空自注册的 Provider
//
// 仅用于测试场景，生产代码不应调用
func ClearAutoRegisteredProviders() {
	autoRegisterMu.Lock()
	defer autoRegisterMu.Unlock()
	autoRegisteredProviders = nil
}
