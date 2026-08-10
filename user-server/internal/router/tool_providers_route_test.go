package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/aiagent/agent/tooluse"
	"hivemtk-user/internal/app"

	"github.com/gin-gonic/gin"
)

// ---- 测试用 Mock Provider（P1-1：原 router 侧 HTTP 用例从 app 测试迁回）----

// providersRouteMockProvider 测试用 Provider
type providersRouteMockProvider struct {
	name  string
	tools []tooluse.Tool
}

func (p *providersRouteMockProvider) Name() string                   { return p.name }
func (p *providersRouteMockProvider) Category() tooluse.ToolCategory { return tooluse.CategoryBusiness }
func (p *providersRouteMockProvider) Description() string            { return "mock provider for route testing" }
func (p *providersRouteMockProvider) Provide(ctx tooluse.ProviderContext) ([]tooluse.Tool, error) {
	return p.tools, nil
}

// ===== HTTP API /api/agent/tools/providers 端到端 =====

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
	// 临时清空全局 ProviderRegistry
	orig := app.GetGlobalProviderRegistry()
	app.SetGlobalProviderRegistryForTest(nil)
	defer app.SetGlobalProviderRegistryForTest(orig)

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
	_ = providerRegistry.RegisterProvider(&providersRouteMockProvider{
		name:  "testp",
		tools: []tooluse.Tool{newMockEchoTool("testp.toola"), newMockEchoTool("testp.toolb")},
	})
	_, _ = providerRegistry.RegisterAll(
		tooluse.ProviderContext{Config: tooluse.ProviderConfig{Enabled: true}},
		tooluse.NewToolRegistry(),
	)

	orig := app.GetGlobalProviderRegistry()
	app.SetGlobalProviderRegistryForTest(providerRegistry)
	defer app.SetGlobalProviderRegistryForTest(orig)

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
