package tooluse

import "sync"


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

