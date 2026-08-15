package tooluse

import (
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
)


// ToolProvider 工具集提供者接口
//
// 一个 Provider 对应一类工具集（如 5 个内置：reach/pm/customer/knowledge/business）
type ToolProvider interface {
	Name() string

	Category() ToolCategory

	Description() string

	Provide(ctx ProviderContext) ([]Tool, error)
}

// ProviderContext 提供者上下文（依赖注入容器）
//
// 通过此结构向 Provider 注入运行时依赖，避免 Provider 直接访问全局状态
type ProviderContext struct {
	DB *gorm.DB

	Config ProviderConfig
}

// ProviderConfig 提供者配置
//
// 支持配置驱动的启停与工具级禁用
type ProviderConfig struct {
	Enabled bool

	DisabledTools []string

	Custom map[string]any
}

// ProviderRegistrationResult 单个 Provider 的注册结果
//
// 由 ProviderRegistry.RegisterAll 返回，用于日志、监控和 /api/agent/tools/providers 接口
type ProviderRegistrationResult struct {
	ProviderName    string        `json:"provider_name"`    
	Category        ToolCategory  `json:"category"`         
	Description     string        `json:"description"`      
	RegisteredTools []string      `json:"registered_tools"` 
	SkippedTools    []string      `json:"skipped_tools"`    
	ToolCount       int           `json:"tool_count"`       
	Skipped         bool          `json:"skipped"`          
	SkippedReason   string        `json:"skipped_reason"`   
	Err             string        `json:"err,omitempty"`    
	Duration        time.Duration `json:"duration_ms"`      
}

// ErrProviderAlreadyExists 重复注册 Provider 错误
var ErrProviderAlreadyExists = fmt.Errorf("tool provider already exists")

// ErrProviderNotFound Provider 不存在错误
var ErrProviderNotFound = fmt.Errorf("tool provider not found")

// ProviderRegistry 提供者注册中心
//
// 管理 ToolProvider 的注册、注销、批量装配
// 线程安全（sync.RWMutex 保护）
type ProviderRegistry struct {
	providers map[string]ToolProvider
	order     []string 
	mu        sync.RWMutex
	results   []ProviderRegistrationResult 
}

// NewProviderRegistry 创建空的 ProviderRegistry
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]ToolProvider),
	}
}

// RegisterProvider 注册一个 Provider
//
// 重复注册同名 Provider 返回 ErrProviderAlreadyExists
func (r *ProviderRegistry) RegisterProvider(p ToolProvider) error {
	if p == nil {
		return fmt.Errorf("provider is nil")
	}
	if p.Name() == "" {
		return fmt.Errorf("provider name is empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[p.Name()]; exists {
		return fmt.Errorf("%w: %s", ErrProviderAlreadyExists, p.Name())
	}
	r.providers[p.Name()] = p
	r.order = append(r.order, p.Name())
	return nil
}

// MustRegisterProvider 注册 Provider，出错 panic
func (r *ProviderRegistry) MustRegisterProvider(p ToolProvider) {
	if err := r.RegisterProvider(p); err != nil {
		panic(err)
	}
}

// UnregisterProvider 注销一个 Provider
//
// 注意：此操作不会从 ToolRegistry 中删除已注册的工具
// 若需同步删除工具，请在调用方手动处理
func (r *ProviderRegistry) UnregisterProvider(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[name]; !exists {
		return fmt.Errorf("%w: %s", ErrProviderNotFound, name)
	}
	delete(r.providers, name)
	for i, n := range r.order {
		if n == name {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
	return nil
}

// HasProvider 判断 Provider 是否已注册
func (r *ProviderRegistry) HasProvider(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.providers[name]
	return exists
}

// GetProvider 获取 Provider
func (r *ProviderRegistry) GetProvider(name string) (ToolProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, exists := r.providers[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, name)
	}
	return p, nil
}

// ListProviders 按注册顺序返回所有 Provider
func (r *ProviderRegistry) ListProviders() []ToolProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ToolProvider, 0, len(r.order))
	for _, name := range r.order {
		if p, ok := r.providers[name]; ok {
			out = append(out, p)
		}
	}
	return out
}

// Count 返回已注册 Provider 数量
func (r *ProviderRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.providers)
}

// RegisterAll 批量装配所有 Provider 到 ToolRegistry
//
// 流程：
//  1. 按注册顺序遍历 Provider
//  2. 检查 ctx.Config.Enabled（默认 true）
//  3. 调用 Provider.Provide(ctx) 获取 []Tool
//  4. 跳过 DisabledTools 命中的工具
//  5. 调用 toolRegistry.Register(t) 注册
//  6. 记录 ProviderRegistrationResult
//
// 返回：
//   - []ProviderRegistrationResult：每个 Provider 的注册结果
//   - error：整体装配过程中是否发生致命错误（单个 Provider 失败不会中断）
func (r *ProviderRegistry) RegisterAll(ctx ProviderContext, toolRegistry *ToolRegistry) ([]ProviderRegistrationResult, error) {
	if toolRegistry == nil {
		return nil, fmt.Errorf("tool registry is nil")
	}
	r.mu.Lock()
	providers := make([]ToolProvider, 0, len(r.order))
	for _, name := range r.order {
		if p, ok := r.providers[name]; ok {
			providers = append(providers, p)
		}
	}
	r.mu.Unlock()

	results := make([]ProviderRegistrationResult, 0, len(providers))
	totalRegistered := 0
	totalSkipped := 0

	for _, p := range providers {
		result := ProviderRegistrationResult{
			ProviderName: p.Name(),
			Category:     p.Category(),
			Description:  p.Description(),
		}

		start := time.Now()

		configProvided := ctx.Config.Custom != nil || len(ctx.Config.DisabledTools) > 0 || ctx.Config.Enabled
		if configProvided && !ctx.Config.Enabled {
			result.Skipped = true
			result.SkippedReason = "disabled by config"
			result.Duration = time.Since(start)
			results = append(results, result)
			totalSkipped++
			continue
		}

		tools, err := p.Provide(ctx)
		if err != nil {
			result.Err = fmt.Sprintf("Provide failed: %v", err)
			result.Duration = time.Since(start)
			results = append(results, result)
			continue
		}

		disabledSet := make(map[string]bool, len(ctx.Config.DisabledTools))
		for _, name := range ctx.Config.DisabledTools {
			disabledSet[name] = true
		}

		for _, t := range tools {
			if disabledSet[t.Name()] {
				result.SkippedTools = append(result.SkippedTools, t.Name())
				continue
			}
			if err := toolRegistry.Register(t); err != nil {
				result.Err = fmt.Sprintf("register tool %s failed: %v; %s", t.Name(), err, result.Err)
				continue
			}
			result.RegisteredTools = append(result.RegisteredTools, t.Name())
		}
		result.ToolCount = len(result.RegisteredTools)
		result.Duration = time.Since(start)
		results = append(results, result)
		totalRegistered += result.ToolCount
	}

	r.mu.Lock()
	r.results = results
	r.mu.Unlock()

	allFailed := true
	for _, r := range results {
		if r.Err == "" && !r.Skipped && r.ToolCount > 0 {
			allFailed = false
			break
		}
	}
	if allFailed && len(results) > 0 {
		return results, fmt.Errorf("all providers failed or skipped")
	}

	_ = totalRegistered
	_ = totalSkipped
	return results, nil
}

// Results 返回最近一次 RegisterAll 的结果
//
// 用于 /api/agent/tools/providers 接口展示
func (r *ProviderRegistry) Results() []ProviderRegistrationResult {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ProviderRegistrationResult, len(r.results))
	copy(out, r.results)
	return out
}

