package tooluse

import (
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
)

// ============================================================================
// ToolProvider 统一扩展入口（+ 优化）
// ----------------------------------------------------------------------------
// 设计目标：为工具链提供"插件式"扩展入口。每个 Provider 对应一类工具集
// （如 reach/customer/knowledge/business/pm 或第三方自定义工具集），
// 通过实现 ToolProvider 接口即可接入系统，无需修改核心装配代码
// （router.registerAllAgentTools）。
//
// 使用方式：
//
//	// 方式 1：实现接口 + 在 router.Setup 中显式注册
//	type MyProvider struct{}
//	func (p *MyProvider) Name() string { return "my" }
//	func (p *MyProvider) Category() ToolCategory { return CategoryBusiness }
//	func (p *MyProvider) Description() string { return "我的工具集" }
//	func (p *MyProvider) Provide(ctx ProviderContext) ([]Tool, error) {
//	    return []Tool{NewMyTool()}, nil
//	}
//	// 在 router 包中：
//	providerRegistry.RegisterProvider(&MyProvider{})
//
//	// 方式 2：实现接口 + 在包 init() 中自注册（推荐第三方包使用）
//	func init() {
//	    tooluse.RegisterToolProvider(&MyProvider{})
//	}
//	// router.Setup 启动时会自动装配所有自注册的 Provider
//
// 扩展点对比（与开源方案）：
//   - LangChain ToolKit：通过 tools=[...] 列表传入，无 Provider 抽象
//   - MCP server：通过协议层暴露 tools/list + tools/call
//   - 本系统 ToolProvider：抽象"工具集"维度，支持配置驱动启停 + 自注册
// ============================================================================

// ToolProvider 工具集提供者接口
//
// 一个 Provider 对应一类工具集（如 5 个内置：reach/pm/customer/knowledge/business）
type ToolProvider interface {
	// Name 提供者唯一名称（如 "reach", "customer", "my_custom"）
	// 用作 ProviderRegistry 的 key；重复注册会返回 ErrProviderAlreadyExists
	Name() string

	// Category 工具分类
	// 所有 Provide 返回的工具都应属于此分类（用于 ListByCategory 查询）
	Category() ToolCategory

	// Description 提供者描述
	// 用于 /api/agent/tools/providers 接口展示，便于运维识别
	Description() string

	// Provide 工厂方法：返回该 Provider 提供的所有工具
	// ctx 携带 DB、Config 等依赖；调用方应保证 ctx.DB 非 nil
	// 返回的 []Tool 不会立即注册到 ToolRegistry，由 ProviderRegistry.RegisterAll 统一注册
	Provide(ctx ProviderContext) ([]Tool, error)
}

// ProviderContext 提供者上下文（依赖注入容器）
//
// 通过此结构向 Provider 注入运行时依赖，避免 Provider 直接访问全局状态
type ProviderContext struct {
	// DB 全局 GORM DB 连接（必填）
	// Provider 应基于此构造 repository / service 依赖
	DB *gorm.DB

	// Config 提供者配置（可选，由配置文件或代码注入）
	Config ProviderConfig
}

// ProviderConfig 提供者配置
//
// 支持配置驱动的启停与工具级禁用
type ProviderConfig struct {
	// Enabled 是否启用该 Provider（默认 true）
	// 设为 false 时 ProviderRegistry.RegisterAll 会跳过此 Provider
	Enabled bool

	// DisabledTools 该 Provider 内需要禁用的具体工具名列表
	// ProviderRegistry.RegisterAll 会跳过此列表中的工具
	DisabledTools []string

	// Custom 自定义配置（由 Provider 自行解析）
	// 例如 reach Provider 可用此传入渠道限流配置
	Custom map[string]any
}

// ProviderRegistrationResult 单个 Provider 的注册结果
//
// 由 ProviderRegistry.RegisterAll 返回，用于日志、监控和 /api/agent/tools/providers 接口
type ProviderRegistrationResult struct {
	ProviderName    string        `json:"provider_name"`    // Provider 名称
	Category        ToolCategory  `json:"category"`         // 工具分类
	Description     string        `json:"description"`      // Provider 描述
	RegisteredTools []string      `json:"registered_tools"` // 成功注册的工具名列表
	SkippedTools    []string      `json:"skipped_tools"`    // 跳过的工具名（DisabledTools 命中）
	ToolCount       int           `json:"tool_count"`       // 成功注册工具数
	Skipped         bool          `json:"skipped"`          // 整体是否被跳过（Enabled=false）
	SkippedReason   string        `json:"skipped_reason"`   // 跳过原因
	Err             string        `json:"err,omitempty"`    // 错误信息（若有）
	Duration        time.Duration `json:"duration_ms"`      // 注册耗时（毫秒）
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
	order     []string // 注册顺序（保证 RegisterAll 顺序可预测）
	mu        sync.RWMutex
	results   []ProviderRegistrationResult // 最近一次 RegisterAll 的结果
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
	// 从 order 中移除
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

		// 检查是否启用
		// 注意：Enabled 默认为 true（零值 false 需要 Provider 显式设置）
		// 这里采用约定：调用方应在 ctx.Config 中显式设置 Enabled
		// 若 ctx.Config 为零值（Enabled=false），则视为未配置，默认启用
		// 通过 Custom 字段是否有值判断是否显式配置
		configProvided := ctx.Config.Custom != nil || len(ctx.Config.DisabledTools) > 0 || ctx.Config.Enabled
		if configProvided && !ctx.Config.Enabled {
			result.Skipped = true
			result.SkippedReason = "disabled by config"
			result.Duration = time.Since(start)
			results = append(results, result)
			totalSkipped++
			continue
		}

		// 调用工厂方法
		tools, err := p.Provide(ctx)
		if err != nil {
			result.Err = fmt.Sprintf("Provide failed: %v", err)
			result.Duration = time.Since(start)
			results = append(results, result)
			continue
		}

		// 注册工具（跳过 DisabledTools）
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
				// 单个工具注册失败不中断，记录到 Err
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

	// 保存结果供后续查询
	r.mu.Lock()
	r.results = results
	r.mu.Unlock()

	// 整体错误仅在所有 Provider 都失败时返回
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
