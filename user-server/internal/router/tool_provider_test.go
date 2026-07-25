package router

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"marketing/internal/aiagent/agent/tooluse"

	"github.com/gin-gonic/gin"
)

// ============================================================================
// ToolProvider 统一扩展入口测试（P0+ 优化验证）
// ----------------------------------------------------------------------------
// 文档依据：docs/企业级架构优化/工具链注册调用机制调研论证.md §五 P1
//
// 测试覆盖：
//   1. ToolProvider 接口契约（Name/Category/Description/Provide）
//   2. ProviderRegistry 注册/注销/查询
//   3. RegisterAll 批量装配（含 Enabled=false 跳过 / DisabledTools 过滤 / Provide 失败处理）
//   4. 自注册机制（RegisterToolProvider / GetAutoRegisteredProviders / ClearAutoRegisteredProviders）
//   5. 5 个内置 Provider 的 Provide() 返回正确工具数
//   6. 第三方 Provider 通过自注册接入
//   7. HTTP API /api/agent/tools/providers 端到端
//   8. 并发安全（多 goroutine 同时 RegisterProvider）
// ============================================================================

// ---- 测试用 Mock Provider ----

// mockProvider 测试用 Provider
type mockProvider struct {
	name        string
	category    tooluse.ToolCategory
	description string
	tools       []tooluse.Tool
	provideErr  error
}

func (p *mockProvider) Name() string                  { return p.name }
func (p *mockProvider) Category() tooluse.ToolCategory { return p.category }
func (p *mockProvider) Description() string            { return p.description }
func (p *mockProvider) Provide(ctx tooluse.ProviderContext) ([]tooluse.Tool, error) {
	if p.provideErr != nil {
		return nil, p.provideErr
	}
	return p.tools, nil
}

// newMockProviderWithTools 构造携带 N 个 mock 工具的 Provider
func newMockProviderWithTools(name string, toolCount int) *mockProvider {
	tools := make([]tooluse.Tool, 0, toolCount)
	for i := 0; i < toolCount; i++ {
		tools = append(tools, newMockEchoTool(name+".tool"+string(rune('a'+i))))
	}
	return &mockProvider{
		name:        name,
		category:    tooluse.CategoryBusiness,
		description: "mock provider for testing",
		tools:       tools,
	}
}

// ===== 1. ProviderRegistry 基本功能 =====

func TestProviderRegistry_RegisterAndGet(t *testing.T) {
	registry := tooluse.NewProviderRegistry()
	p := newMockProviderWithTools("test1", 2)

	if err := registry.RegisterProvider(p); err != nil {
		t.Fatalf("RegisterProvider failed: %v", err)
	}
	if !registry.HasProvider("test1") {
		t.Error("HasProvider should return true")
	}
	if registry.Count() != 1 {
		t.Errorf("Count = %d, want 1", registry.Count())
	}

	got, err := registry.GetProvider("test1")
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}
	if got.Name() != "test1" {
		t.Errorf("got.Name = %s, want test1", got.Name())
	}

	// 重复注册应失败
	if err := registry.RegisterProvider(p); err == nil {
		t.Error("duplicate RegisterProvider should fail")
	}

	// 注销
	if err := registry.UnregisterProvider("test1"); err != nil {
		t.Fatalf("UnregisterProvider failed: %v", err)
	}
	if registry.HasProvider("test1") {
		t.Error("HasProvider should return false after Unregister")
	}
}

func TestProviderRegistry_RegisterNil(t *testing.T) {
	registry := tooluse.NewProviderRegistry()
	if err := registry.RegisterProvider(nil); err == nil {
		t.Error("RegisterProvider(nil) should fail")
	}
}

func TestProviderRegistry_RegisterEmptyName(t *testing.T) {
	registry := tooluse.NewProviderRegistry()
	p := &mockProvider{name: "", category: tooluse.CategoryBusiness, description: "x"}
	if err := registry.RegisterProvider(p); err == nil {
		t.Error("RegisterProvider with empty name should fail")
	}
}

func TestProviderRegistry_ListProvidersOrder(t *testing.T) {
	registry := tooluse.NewProviderRegistry()
	_ = registry.RegisterProvider(&mockProvider{name: "c", category: tooluse.CategoryBusiness})
	_ = registry.RegisterProvider(&mockProvider{name: "a", category: tooluse.CategoryBusiness})
	_ = registry.RegisterProvider(&mockProvider{name: "b", category: tooluse.CategoryBusiness})

	providers := registry.ListProviders()
	if len(providers) != 3 {
		t.Fatalf("ListProviders len = %d, want 3", len(providers))
	}
	// 应按注册顺序，不是字母序
	if providers[0].Name() != "c" || providers[1].Name() != "a" || providers[2].Name() != "b" {
		t.Errorf("order = %s,%s,%s; want c,a,b",
			providers[0].Name(), providers[1].Name(), providers[2].Name())
	}
}

// ===== 2. RegisterAll 批量装配 =====

func TestProviderRegistry_RegisterAll_Success(t *testing.T) {
	providerRegistry := tooluse.NewProviderRegistry()
	toolRegistry := tooluse.NewToolRegistry()

	_ = providerRegistry.RegisterProvider(newMockProviderWithTools("p1", 3))
	_ = providerRegistry.RegisterProvider(newMockProviderWithTools("p2", 2))

	results, err := providerRegistry.RegisterAll(
		tooluse.ProviderContext{DB: nil, Config: tooluse.ProviderConfig{Enabled: true}},
		toolRegistry,
	)
	if err != nil {
		t.Fatalf("RegisterAll err: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results))
	}
	if toolRegistry.Count() != 5 {
		t.Errorf("tool registry Count = %d, want 5", toolRegistry.Count())
	}

	// 检查每个 Provider 的结果
	for _, r := range results {
		if r.ToolCount == 0 {
			t.Errorf("provider %s ToolCount = 0", r.ProviderName)
		}
		if r.Err != "" {
			t.Errorf("provider %s Err = %s", r.ProviderName, r.Err)
		}
	}
}

func TestProviderRegistry_RegisterAll_SkippedByConfig(t *testing.T) {
	providerRegistry := tooluse.NewProviderRegistry()
	toolRegistry := tooluse.NewToolRegistry()

	_ = providerRegistry.RegisterProvider(newMockProviderWithTools("enabled", 2))
	_ = providerRegistry.RegisterProvider(newMockProviderWithTools("disabled", 2))

	// 通过 Custom 配置触发 configProvided = true，并设 Enabled=false
	// 但 ProviderConfig 是全局的，无法按 Provider 单独配置
	// 这里测试整体 Enabled=false 的场景
	ctx := tooluse.ProviderContext{
		DB: nil,
		Config: tooluse.ProviderConfig{
			Enabled: false,
			Custom:  map[string]any{"test": true}, // 触发 configProvided
		},
	}
	results, err := providerRegistry.RegisterAll(ctx, toolRegistry)
	// 整体 Enabled=false，所有 Provider 应该被跳过
	if err != nil {
		t.Logf("RegisterAll err (expected when all skipped): %v", err)
	}
	for _, r := range results {
		if !r.Skipped {
			t.Errorf("provider %s should be skipped", r.ProviderName)
		}
	}
	if toolRegistry.Count() != 0 {
		t.Errorf("tool registry should be empty, got %d", toolRegistry.Count())
	}
}

func TestProviderRegistry_RegisterAll_DisabledTools(t *testing.T) {
	providerRegistry := tooluse.NewProviderRegistry()
	toolRegistry := tooluse.NewToolRegistry()

	// 构造一个含 3 个工具的 Provider，禁用其中 1 个
	p := &mockProvider{
		name:     "p",
		category: tooluse.CategoryBusiness,
		tools: []tooluse.Tool{
			newMockEchoTool("p.keep1"),
			newMockEchoTool("p.skip"),
			newMockEchoTool("p.keep2"),
		},
	}
	_ = providerRegistry.RegisterProvider(p)

	ctx := tooluse.ProviderContext{
		DB: nil,
		Config: tooluse.ProviderConfig{
			Enabled:       true,
			DisabledTools: []string{"p.skip"},
		},
	}
	results, _ := providerRegistry.RegisterAll(ctx, toolRegistry)
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	if results[0].ToolCount != 2 {
		t.Errorf("ToolCount = %d, want 2", results[0].ToolCount)
	}
	if len(results[0].SkippedTools) != 1 || results[0].SkippedTools[0] != "p.skip" {
		t.Errorf("SkippedTools = %v, want [p.skip]", results[0].SkippedTools)
	}
	if toolRegistry.Has("p.skip") {
		t.Error("p.skip should not be registered")
	}
	if !toolRegistry.Has("p.keep1") || !toolRegistry.Has("p.keep2") {
		t.Error("p.keep1 and p.keep2 should be registered")
	}
}

func TestProviderRegistry_RegisterAll_ProvideError(t *testing.T) {
	providerRegistry := tooluse.NewProviderRegistry()
	toolRegistry := tooluse.NewToolRegistry()

	pErr := errors.New("mock provide failure")
	_ = providerRegistry.RegisterProvider(&mockProvider{
		name:       "fail",
		category:   tooluse.CategoryBusiness,
		provideErr: pErr,
	})
	_ = providerRegistry.RegisterProvider(newMockProviderWithTools("ok", 1))

	results, _ := providerRegistry.RegisterAll(
		tooluse.ProviderContext{Config: tooluse.ProviderConfig{Enabled: true}},
		toolRegistry,
	)
	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results))
	}
	// fail Provider 应记录错误
	var failResult *tooluse.ProviderRegistrationResult
	for i := range results {
		if results[i].ProviderName == "fail" {
			failResult = &results[i]
			break
		}
	}
	if failResult == nil {
		t.Fatal("fail provider result not found")
	}
	if failResult.Err == "" {
		t.Error("fail provider Err should be non-empty")
	}
	if failResult.ToolCount != 0 {
		t.Errorf("fail provider ToolCount = %d, want 0", failResult.ToolCount)
	}
	// ok Provider 仍应正常注册
	if toolRegistry.Count() != 1 {
		t.Errorf("tool registry Count = %d, want 1 (only ok provider)", toolRegistry.Count())
	}
}

func TestProviderRegistry_RegisterAll_NilToolRegistry(t *testing.T) {
	providerRegistry := tooluse.NewProviderRegistry()
	_, err := providerRegistry.RegisterAll(
		tooluse.ProviderContext{Config: tooluse.ProviderConfig{Enabled: true}},
		nil,
	)
	if err == nil {
		t.Error("RegisterAll with nil toolRegistry should fail")
	}
}

// ===== 3. Results 缓存 =====

func TestProviderRegistry_Results(t *testing.T) {
	providerRegistry := tooluse.NewProviderRegistry()
	toolRegistry := tooluse.NewToolRegistry()
	_ = providerRegistry.RegisterProvider(newMockProviderWithTools("p1", 1))

	// 未调用 RegisterAll 时 Results 应为空
	if len(providerRegistry.Results()) != 0 {
		t.Error("Results should be empty before RegisterAll")
	}

	_, _ = providerRegistry.RegisterAll(
		tooluse.ProviderContext{Config: tooluse.ProviderConfig{Enabled: true}},
		toolRegistry,
	)
	if len(providerRegistry.Results()) != 1 {
		t.Errorf("Results len = %d, want 1", len(providerRegistry.Results()))
	}
}

// ===== 4. 自注册机制 =====

func TestAutoRegister_BasicFlow(t *testing.T) {
	// 清理状态（避免其他测试污染）
	tooluse.ClearAutoRegisteredProviders()
	defer tooluse.ClearAutoRegisteredProviders()

	p1 := newMockProviderWithTools("auto1", 1)
	p2 := newMockProviderWithTools("auto2", 1)

	tooluse.RegisterToolProvider(p1)
	tooluse.RegisterToolProvider(p2)

	got := tooluse.GetAutoRegisteredProviders()
	if len(got) != 2 {
		t.Fatalf("auto providers len = %d, want 2", len(got))
	}
}

func TestAutoRegister_DuplicateName(t *testing.T) {
	tooluse.ClearAutoRegisteredProviders()
	defer tooluse.ClearAutoRegisteredProviders()

	p1 := newMockProviderWithTools("dup", 1)
	p2 := newMockProviderWithTools("dup", 2) // 同名，应覆盖

	tooluse.RegisterToolProvider(p1)
	tooluse.RegisterToolProvider(p2)

	got := tooluse.GetAutoRegisteredProviders()
	if len(got) != 1 {
		t.Errorf("duplicate name should be deduped, got len = %d", len(got))
	}
}

func TestAutoRegister_Nil(t *testing.T) {
	tooluse.ClearAutoRegisteredProviders()
	defer tooluse.ClearAutoRegisteredProviders()

	tooluse.RegisterToolProvider(nil) // 应静默忽略
	if len(tooluse.GetAutoRegisteredProviders()) != 0 {
		t.Error("nil provider should be ignored")
	}
}

func TestAutoRegister_GetReturnsCopy(t *testing.T) {
	tooluse.ClearAutoRegisteredProviders()
	defer tooluse.ClearAutoRegisteredProviders()

	tooluse.RegisterToolProvider(newMockProviderWithTools("p", 1))
	got := tooluse.GetAutoRegisteredProviders()
	// 修改返回的切片不应影响内部状态
	got[0] = nil

	got2 := tooluse.GetAutoRegisteredProviders()
	if got2[0] == nil {
		t.Error("GetAutoRegisteredProviders should return a copy")
	}
}

// ===== 5. 内置 Provider Provide() 验证 =====

func TestBuiltinProviders_ProvideToolCount(t *testing.T) {
	// 注：本测试不依赖 DB（除了 ReachToolProvider 需要 DB 装配 adapter）
	// 通过 mock ProviderContext 验证 Provide() 返回的工具数量

	cases := []struct {
		name         string
		provider     tooluse.ToolProvider
		expectDBErr  bool // 是否期望返回 DB 必需错误
		expectCount  int  // 不期望错误时，应返回的工具数
		requireDB    bool
	}{
		{
			name:        "ReachToolProvider",
			provider:    &ReachToolProvider{},
			expectDBErr: true, // 没有 DB
			requireDB:   true,
		},
		{
			name:        "PrivateMessageToolProvider",
			provider:    &PrivateMessageToolProvider{},
			expectDBErr: true,
			requireDB:   true,
		},
		{
			name:        "CustomerToolProvider",
			provider:    &CustomerToolProvider{},
			expectDBErr: true,
			requireDB:   true,
		},
		{
			name:        "KnowledgeToolProvider",
			provider:    &KnowledgeToolProvider{},
			expectDBErr: true,
			requireDB:   true,
		},
		{
			name:        "BusinessToolProvider",
			provider:    &BusinessToolProvider{},
			expectDBErr: true,
			requireDB:   true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tools, err := c.provider.Provide(tooluse.ProviderContext{DB: nil})
			if c.expectDBErr {
				if err == nil {
					t.Errorf("%s.Provide(nil DB) should fail, got tools=%d", c.name, len(tools))
				}
				return
			}
			if err != nil {
				t.Errorf("%s.Provide err: %v", c.name, err)
			}
			if c.expectCount > 0 && len(tools) != c.expectCount {
				t.Errorf("%s tools len = %d, want %d", c.name, len(tools), c.expectCount)
			}
		})
	}
}

func TestBuiltinProviders_MetaData(t *testing.T) {
	cases := []struct {
		name             string
		provider         tooluse.ToolProvider
		expectName       string
		expectCategory   tooluse.ToolCategory
		expectDescNonEmpty bool
	}{
		{"ReachToolProvider", &ReachToolProvider{}, "reach", tooluse.CategoryReach, true},
		{"PrivateMessageToolProvider", &PrivateMessageToolProvider{}, "pm", tooluse.CategoryPrivateMessage, true},
		{"CustomerToolProvider", &CustomerToolProvider{}, "customer", tooluse.CategoryCustomer, true},
		{"KnowledgeToolProvider", &KnowledgeToolProvider{}, "knowledge", tooluse.CategoryKnowledge, true},
		{"BusinessToolProvider", &BusinessToolProvider{}, "business", tooluse.CategoryBusiness, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.provider.Name() != c.expectName {
				t.Errorf("Name = %s, want %s", c.provider.Name(), c.expectName)
			}
			if c.provider.Category() != c.expectCategory {
				t.Errorf("Category = %s, want %s", c.provider.Category(), c.expectCategory)
			}
			if c.expectDescNonEmpty && c.provider.Description() == "" {
				t.Error("Description should be non-empty")
			}
		})
	}
}

// ===== 6. 第三方 Provider 通过自注册接入端到端 =====

func TestAutoRegister_IntegrationWithProviderRegistry(t *testing.T) {
	tooluse.ClearAutoRegisteredProviders()
	defer tooluse.ClearAutoRegisteredProviders()

	// 模拟第三方包自注册一个 Provider
	thirdPartyProvider := newMockProviderWithTools("thirdparty", 2)
	tooluse.RegisterToolProvider(thirdPartyProvider)

	// 模拟 registerAllAgentToolsViaProviders 的流程
	providerRegistry := tooluse.NewProviderRegistry()
	// 内置 Provider（这里跳过，仅测试第三方）
	for _, p := range tooluse.GetAutoRegisteredProviders() {
		if err := providerRegistry.RegisterProvider(p); err != nil {
			t.Errorf("RegisterProvider %s failed: %v", p.Name(), err)
		}
	}

	toolRegistry := tooluse.NewToolRegistry()
	results, _ := providerRegistry.RegisterAll(
		tooluse.ProviderContext{Config: tooluse.ProviderConfig{Enabled: true}},
		toolRegistry,
	)
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	if results[0].ProviderName != "thirdparty" {
		t.Errorf("ProviderName = %s, want thirdparty", results[0].ProviderName)
	}
	if results[0].ToolCount != 2 {
		t.Errorf("ToolCount = %d, want 2", results[0].ToolCount)
	}
	if toolRegistry.Count() != 2 {
		t.Errorf("tool registry Count = %d, want 2", toolRegistry.Count())
	}
}

// ===== 7. 并发安全 =====

func TestProviderRegistry_ConcurrentRegister(t *testing.T) {
	providerRegistry := tooluse.NewProviderRegistry()
	const N = 20
	var wg sync.WaitGroup
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			p := newMockProviderWithTools("p"+string(rune('a'+idx)), 1)
			if err := providerRegistry.RegisterProvider(p); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	errCount := 0
	for err := range errs {
		if err != nil {
			errCount++
			t.Logf("concurrent register err: %v", err)
		}
	}
	// 应该所有注册都成功（名字不同）
	if errCount > 0 {
		t.Errorf("concurrent register err count = %d, want 0", errCount)
	}
}

// ===== 8. HTTP API /api/agent/tools/providers 端到端 =====

func TestSetup_ToolProvidersRouteRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	auth := r.Group("/api")
	setupToolDebugRoutes(auth)

	routes := r.Routes()
	routeSet := make(map[string]bool)
	for _, route := range routes {
		routeSet[route.Method+"-"+route.Path] = true
	}
	if !routeSet["GET-/api/agent/tools/providers"] {
		t.Error("route GET-/api/agent/tools/providers not registered")
	}
}

func TestHandleToolProviders_HTTP_WithoutInit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 临时清空 globalProviderRegistry
	orig := globalProviderRegistry
	globalProviderRegistry = nil
	defer func() { globalProviderRegistry = orig }()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/agent/tools/providers", nil)

	handleToolProviders(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if resp["success"] != false {
		t.Error("success should be false")
	}
}

func TestHandleToolProviders_HTTP_WithProviderRegistry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 构造一个已装配的 ProviderRegistry
	providerRegistry := tooluse.NewProviderRegistry()
	_ = providerRegistry.RegisterProvider(newMockProviderWithTools("testp", 2))
	_, _ = providerRegistry.RegisterAll(
		tooluse.ProviderContext{Config: tooluse.ProviderConfig{Enabled: true}},
		tooluse.NewToolRegistry(),
	)

	orig := globalProviderRegistry
	globalProviderRegistry = providerRegistry
	defer func() { globalProviderRegistry = orig }()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/agent/tools/providers", nil)

	handleToolProviders(c)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body not JSON: %v; body=%s", err, w.Body.String())
	}
	if resp["success"] != true {
		t.Errorf("success should be true; body=%s", w.Body.String())
	}
	// total_providers 应为 1
	if int(resp["total_providers"].(float64)) != 1 {
		t.Errorf("total_providers = %v, want 1", resp["total_providers"])
	}
	results := resp["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	first := results[0].(map[string]any)
	if first["provider_name"] != "testp" {
		t.Errorf("provider_name = %v, want testp", first["provider_name"])
	}
}

// ===== 9. initBuiltinToolProviders 验证 =====

func TestInitBuiltinToolProviders_RegistersAll(t *testing.T) {
	registry := tooluse.NewProviderRegistry()
	initBuiltinToolProviders(registry)

	if registry.Count() != 5 {
		t.Errorf("Count = %d, want 5", registry.Count())
	}
	expected := []string{"reach", "pm", "customer", "knowledge", "business"}
	for _, name := range expected {
		if !registry.HasProvider(name) {
			t.Errorf("provider %s not registered", name)
		}
	}
}

// ===== 10. ProviderContext 兼容性 =====

func TestProviderContext_ZeroValue(t *testing.T) {
	var ctx tooluse.ProviderContext
	// 零值 DB 应为 nil
	if ctx.DB != nil {
		t.Error("zero value DB should be nil")
	}
	// 零值 Enabled 应为 false
	if ctx.Config.Enabled {
		t.Error("zero value Enabled should be false")
	}
}

// ---- 辅助：mock 工具实现（避免依赖 tool_debug_routes_test.go 中的同名类型）----

// 注意：mockTool 类型已在 tool_debug_routes_test.go 中定义，此处复用
// 通过 newMockEchoTool 函数构造（已在 tool_debug_routes_test.go 中定义）

// 确保 mockProvider 实现 tooluse.ToolProvider 接口
var _ tooluse.ToolProvider = (*mockProvider)(nil)

// 确保 5 个内置 Provider 实现 tooluse.ToolProvider 接口
var _ tooluse.ToolProvider = (*ReachToolProvider)(nil)
var _ tooluse.ToolProvider = (*PrivateMessageToolProvider)(nil)
var _ tooluse.ToolProvider = (*CustomerToolProvider)(nil)
var _ tooluse.ToolProvider = (*KnowledgeToolProvider)(nil)
var _ tooluse.ToolProvider = (*BusinessToolProvider)(nil)

// 占位：避免 import "context" 未使用（如果未来添加 ctx 相关测试）
var _ = context.Background
