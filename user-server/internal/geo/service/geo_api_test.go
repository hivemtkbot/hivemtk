package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/service"
	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/router"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func geoAPITestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.GeoKeyword{},
		&model.GeoArticle{},
		&model.GeoOptimization{},
		&model.GeoVerifyResult{},
		&model.GeoAPICall{},
		&model.GeoConfig{},
		&model.GeoPlatformAccount{},
		&model.GeoPublishRecord{},
		&model.GeoKnowledgeDocument{},
		&model.GeoWorkflow{},
		&model.GeoWorkflowExecution{},
		&model.GeoWorkflowTemplate{},
	)
}

func setupGeoRouter(t *testing.T) *gin.Engine {
	db := geoAPITestDB(t)
	gin.SetMode(gin.TestMode)

	middleware.IsTestMode = true
	t.Cleanup(func() { middleware.IsTestMode = false })
	r := gin.New()
	auth := r.Group("/api")
	auth.Use(func(c *gin.Context) { c.Next() })
	router.SetupGeoRoutes(auth, db)
	return r
}

var (
	llmProbeOnce sync.Once
	llmAvailable bool
)

func requireLLM(t *testing.T) {
	t.Helper()
	llmProbeOnce.Do(func() {

		done := make(chan bool, 1)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			_, err := service.NewLLMAdapter().GenerateJSON(ctx, "", "连接测试，请回复 ok", 10)
			done <- err == nil
		}()
		select {
		case ok := <-done:
			llmAvailable = ok
		case <-time.After(10 * time.Second):
			llmAvailable = false
		}
	})
	if !llmAvailable {
		t.Skip("LLM 后端不可达（未配置 API Key 或网络不通），跳过依赖 LLM 的集成测试")
	}
}

func doRequest(t *testing.T, r http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody string
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = string(b)
	}
	req := httptest.NewRequest(method, "/api"+path, strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func assertStatus(t *testing.T, w *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if w.Code != expected {
		t.Fatalf("expected status %d, got %d: %s", expected, w.Code, w.Body.String())
	}
}

func assertSuccess(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response failed: %v, body: %s", err, w.Body.String())
	}
	code, ok := resp["code"]
	if !ok || code != float64(0) {
		t.Fatalf("expected code=0, got %v (body: %s)", resp["code"], w.Body.String())
	}
}

func TestGeoAPI_GetConfig(t *testing.T) {
	r := setupGeoRouter(t)
	w := doRequest(t, r, "GET", "/geo/config", nil)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)
}

func TestGeoAPI_UpdateConfig(t *testing.T) {
	r := setupGeoRouter(t)
	body := map[string]any{"brand": "测试品牌", "advantages": "技术领先"}
	w := doRequest(t, r, "PUT", "/geo/config", body)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)
}

func TestGeoAPI_MineKeywords(t *testing.T) {
	r := setupGeoRouter(t)
	requireLLM(t)
	body := map[string]any{
		"seed_words": []string{"GEO优化", "AI搜索"},
		"mode":       "industry",
		"brand_name": "测试品牌",
	}
	w := doRequest(t, r, "POST", "/geo/keywords/mine", body)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)
}

func TestGeoAPI_SemanticExpand(t *testing.T) {
	r := setupGeoRouter(t)
	requireLLM(t)
	body := map[string]any{
		"keywords":   []string{"GEO优化", "AI搜索"},
		"brand_name": "测试品牌",
	}
	w := doRequest(t, r, "POST", "/geo/keywords/expand", body)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)
}

func TestGeoAPI_TopicCluster(t *testing.T) {
	r := setupGeoRouter(t)
	requireLLM(t)
	body := map[string]any{
		"keywords":   []string{"GEO优化", "AI搜索", "品牌营销"},
		"brand_name": "测试品牌",
	}
	w := doRequest(t, r, "POST", "/geo/keywords/cluster", body)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)
}

func TestGeoAPI_GetKeywordList(t *testing.T) {
	r := setupGeoRouter(t)
	w := doRequest(t, r, "GET", "/geo/keywords/list?page=1&limit=10", nil)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)
}

func TestGeoAPI_GenerateContent(t *testing.T) {
	r := setupGeoRouter(t)
	requireLLM(t)
	body := map[string]any{
		"keyword":    "GEO优化",
		"brand_name": "测试品牌",
		"advantages": []string{"技术领先", "服务优质"},
		"word_count": 800,
		"style":      "专业",
	}
	w := doRequest(t, r, "POST", "/geo/content/generate", body)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)
}

func TestGeoAPI_ScoreContent(t *testing.T) {
	r := setupGeoRouter(t)
	requireLLM(t)
	body := map[string]any{
		"content":    "根据2024年报告显示，GEO优化能提升品牌曝光率30%。",
		"brand_name": "测试品牌",
		"keyword":    "GEO优化",
	}
	w := doRequest(t, r, "POST", "/geo/content/score", body)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)
}

func TestGeoAPI_CheckUniqueness(t *testing.T) {
	r := setupGeoRouter(t)
	body := map[string]any{
		"content": "这是一段测试内容，用于检测独特性。GEO优化是一种新兴的营销策略。",
	}
	w := doRequest(t, r, "POST", "/geo/content/uniqueness", body)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)
}

func TestGeoAPI_GetArticleList(t *testing.T) {
	r := setupGeoRouter(t)
	w := doRequest(t, r, "GET", "/geo/content/list?page=1&limit=10", nil)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)
}

func TestGeoAPI_VerifyArticle(t *testing.T) {
	r := setupGeoRouter(t)
	requireLLM(t)
	body := map[string]any{
		"brand_name": "测试品牌",
		"query":      "GEO优化怎么样",
		"article_id": "test-article-001",
	}
	w := doRequest(t, r, "POST", "/geo/verification/verify", body)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)
}

func TestGeoAPI_MonitorNegative(t *testing.T) {
	r := setupGeoRouter(t)
	requireLLM(t)
	body := map[string]any{"brand_name": "测试品牌"}
	w := doRequest(t, r, "POST", "/geo/verification/negative", body)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)
}

func TestGeoAPI_KBSaveAndGet(t *testing.T) {
	r := setupGeoRouter(t)

	saveBody := map[string]any{
		"title":    "GEO优化指南",
		"content":  "GEO（生成式引擎优化）是一种针对AI搜索引擎的优化策略...",
		"doc_type": "reference",
	}
	w := doRequest(t, r, "POST", "/geo/kb/documents", saveBody)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)

	var saveResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &saveResp)
	data, _ := saveResp["data"].(map[string]any)
	id, _ := data["id"].(string)
	if id == "" {
		t.Fatal("expected document ID")
	}

	w = doRequest(t, r, "GET", fmt.Sprintf("/geo/kb/documents/%s", id), nil)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)

	w = doRequest(t, r, "GET", "/geo/kb/search?q=GEO&limit=5", nil)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)

	w = doRequest(t, r, "DELETE", fmt.Sprintf("/geo/kb/documents/%s", id), nil)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)
}

func TestGeoAPI_KBList(t *testing.T) {
	r := setupGeoRouter(t)
	w := doRequest(t, r, "GET", "/geo/kb/documents", nil)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)
}

func TestGeoAPI_ListPlatforms(t *testing.T) {
	r := setupGeoRouter(t)
	w := doRequest(t, r, "GET", "/geo/platform/platforms", nil)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := resp["data"].([]any)
	if len(data) == 0 {
		t.Fatal("expected non-empty platform list")
	}
}

func TestGeoAPI_SaveAndListAccounts(t *testing.T) {
	r := setupGeoRouter(t)

	body := map[string]any{
		"platform":     "github_readme",
		"account_name": "test-user",
		"credentials":  map[string]string{"access_token": "ghp_test123"},
	}
	w := doRequest(t, r, "POST", "/geo/platform/accounts", body)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)

	w = doRequest(t, r, "GET", "/geo/platform/accounts", nil)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)
}

func TestGeoAPI_WorkflowCRUD(t *testing.T) {
	r := setupGeoRouter(t)

	body := map[string]any{
		"name": "测试工作流",
		"steps": []map[string]any{
			{"name": "step1", "type": "content_generate", "params": map[string]any{"topic": "GEO"}},
		},
		"enabled": true,
	}
	w := doRequest(t, r, "POST", "/geo/workflow/workflows", body)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := resp["data"].(map[string]any)
	id, _ := data["id"].(string)

	w = doRequest(t, r, "GET", fmt.Sprintf("/geo/workflow/workflows/%s", id), nil)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)

	w = doRequest(t, r, "GET", "/geo/workflow/workflows", nil)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)

	w = doRequest(t, r, "POST", fmt.Sprintf("/geo/workflow/workflows/%s/run", id), nil)
	assertStatus(t, w, http.StatusOK)

	w = doRequest(t, r, "DELETE", fmt.Sprintf("/geo/workflow/workflows/%s", id), nil)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)
}

func TestGeoAPI_WorkflowTemplates(t *testing.T) {
	r := setupGeoRouter(t)

	body := map[string]any{
		"name":        "内容生产模板",
		"description": "自动生成内容的标准工作流",
		"steps": []map[string]any{
			{"name": "generate", "type": "content_generate"},
		},
	}
	w := doRequest(t, r, "POST", "/geo/workflow/templates", body)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)

	w = doRequest(t, r, "GET", "/geo/workflow/templates", nil)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)
}

func TestGeoAPI_TechConfig(t *testing.T) {
	r := setupGeoRouter(t)

	w := doRequest(t, r, "POST", "/geo/techconfig/robots", map[string]any{
		"site_url": "https://example.com",
		"disallow": []string{"/admin", "/api"},
	})
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)

	w = doRequest(t, r, "POST", "/geo/techconfig/sitemap", map[string]any{
		"site_url": "https://example.com",
		"urls":     []string{"/", "/about", "/blog"},
	})
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)
}

func TestGeoAPI_MetricsAnalyze(t *testing.T) {
	r := setupGeoRouter(t)
	body := map[string]any{
		"content": "根据2024年报告显示，GEO优化能提升品牌曝光率30%。例如某品牌使用GEO后搜索量增长了2倍。",
		"keyword": "GEO优化",
		"brand":   "测试品牌",
	}
	w := doRequest(t, r, "POST", "/geo/metrics/analyze", body)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)
}

func TestGeoAPI_Reports(t *testing.T) {
	r := setupGeoRouter(t)

	endpoints := []string{
		"/geo/reports/summary",
		"/geo/reports/roi",
		"/geo/reports/api-costs",
	}
	for _, ep := range endpoints {
		w := doRequest(t, r, "GET", ep, nil)
		assertStatus(t, w, http.StatusOK)
		assertSuccess(t, w)
	}
}

func TestGeoAPI_KeywordEnhance(t *testing.T) {
	r := setupGeoRouter(t)

	w := doRequest(t, r, "GET", "/geo/keyword-enhance/analyze?brand_name=测试品牌", nil)
	assertStatus(t, w, http.StatusOK)
	assertSuccess(t, w)

	body := map[string]any{
		"keywords":   []string{"GEO优化", "AI搜索"},
		"brand_name": "测试品牌",
	}
	w = doRequest(t, r, "POST", "/geo/keyword-enhance/enhance", body)
	assertStatus(t, w, http.StatusOK)
}
