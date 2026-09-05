package llm

import "sync"

var (
	globalDispatcher     *Dispatcher
	globalDispatcherOnce sync.Once
)

// GetGlobalDispatcher 获取全局 Dispatcher 单例。
// 首次调用时基于默认 LLMService 初始化，包含默认厂商与路由策略。
// InitGlobalDispatcher 用配置构建的调度器覆盖全局默认。
// 必须在首次 GetGlobalDispatcher 之前调用（main 启动时注入本地优先调度器）。
func InitGlobalDispatcher(d *Dispatcher) {
	globalDispatcherOnce.Do(func() {
		globalDispatcher = d
	})
}

func GetGlobalDispatcher() *Dispatcher {
	globalDispatcherOnce.Do(func() {
		globalDispatcher = NewDispatcher(NewLLMService())
	})
	return globalDispatcher
}

// RemoveProvider 删除指定厂商配置，返回是否实际删除。
func (d *Dispatcher) RemoveProvider(name string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.providers[name]; !ok {
		return false
	}
	delete(d.providers, name)
	delete(d.rpmCounter, name)
	return true
}

// GetProvider 获取指定厂商配置（不存在返回 nil）。
func (d *Dispatcher) GetProvider(name string) *ProviderConfig {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if p, ok := d.providers[name]; ok {
		cp := *p
		return &cp
	}
	return nil
}

// GetRoute 获取指定场景路由（不存在返回 nil）。
func (d *Dispatcher) GetRoute(scenario DispatchScenario) *ScenarioRoute {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if r, ok := d.routes[scenario]; ok {
		cr := *r
		return &cr
	}
	return nil
}

// RemoveRoute 删除指定场景路由。
func (d *Dispatcher) RemoveRoute(scenario DispatchScenario) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.routes[scenario]; !ok {
		return false
	}
	delete(d.routes, scenario)
	return true
}

// GetCandidateChain 返回场景的降级链（主 provider → 备选）
// 按顺序：route.Provider + route.Fallbacks，自动去重并附加本地兜底 provider
// 用于管理后台展示降级链全貌
func (d *Dispatcher) GetCandidateChain(scenario DispatchScenario) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	route, ok := d.routes[scenario]
	if !ok {
		return nil
	}
	seen := make(map[string]bool)
	out := make([]string, 0, len(route.Fallbacks)+2)
	seen[route.Provider] = true
	out = append(out, route.Provider)
	for _, name := range route.Fallbacks {
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	if !seen["default"] {
		if _, ok := d.providers["default"]; ok {
			out = append(out, "default")
		}
	}
	return out
}

// GetEnabledProviderNames 返回当前启用的 provider 名称列表
func (d *Dispatcher) GetEnabledProviderNames() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	names := make([]string, 0, len(d.providers))
	for _, p := range d.providers {
		if p.Enabled {
			names = append(names, p.Name)
		}
	}
	return names
}

// CountProvidersByStatus 统计 provider 状态（up / down / disabled）
func (d *Dispatcher) CountProvidersByStatus() (up, down, disabled int) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, p := range d.providers {
		if !p.Enabled {
			disabled++
			continue
		}
		up++
	}
	return up, down, disabled
}
