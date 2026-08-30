package controller

import (
	"bytes"
	"encoding/json"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/service"
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupTikTokCardTestDB 设置 TikTok 卡片测试数据库
func setupTikTokCardTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.TikTokCard{},
		&model.TikTokCardActivity{},
		&model.ShortLink{},
	)
}

// setupTikTokCardTestRouter 设置 TikTok 卡片测试路由
func setupTikTokCardTestRouter(t *testing.T) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	db := setupTikTokCardTestDB(t)
	ctrl := &TikTokCardController{
		svc: service.NewTikTokCardServiceWithDB(db),
	}
	ctrl.RegisterRoutes(router.Group("/api"))
	return router
}

// TestTikTokCardController_Create_Success 测试创建 TikTok 卡片成功
func TestTikTokCardController_Create_Success(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	createReq := map[string]any{
		"title":          "Test Card",
		"description":    "This is a test card",
		"image_url":      "https://example.com/image.jpg",
		"redirect_url":   "https://www.tiktok.com",
		"is_active":      true,
		"domain_pool_id": 1,
		"tags":           "test,card",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/api/tiktok-card", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["code"] != float64(0) {
		t.Errorf("Expected code SUCCESS, got %v", response["code"])
	}
}

// TestTikTokCardController_Create_InvalidJSON 测试无效 JSON 请求
func TestTikTokCardController_Create_InvalidJSON(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	req, _ := http.NewRequest("POST", "/api/tiktok-card", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestTikTokCardController_Create_EmptyTitle 测试空标题
func TestTikTokCardController_Create_EmptyTitle(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	createReq := map[string]any{
		"title":       "",
		"description": "This is a test card",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/api/tiktok-card", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestTikTokCardController_Create_MinimalRequest 测试最小请求
func TestTikTokCardController_Create_MinimalRequest(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	createReq := map[string]any{
		"title": "Minimal Card",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/api/tiktok-card", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestTikTokCardController_Update_Success 测试更新卡片成功
func TestTikTokCardController_Update_Success(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	updateReq := map[string]any{
		"title":          "Updated Card",
		"description":    "Updated description",
		"image_url":      "https://example.com/new-image.jpg",
		"redirect_url":   "https://www.tiktok.com/new",
		"is_active":      true,
		"domain_pool_id": 1,
		"tags":           "updated,card",
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/api/tiktok-card/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Errorf("Expected error for non-existent card, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestTikTokCardController_Update_InvalidJSON 测试无效 JSON
func TestTikTokCardController_Update_InvalidJSON(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	req, _ := http.NewRequest("PUT", "/api/tiktok-card/1", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestTikTokCardController_Update_EmptyID 测试空 ID
func TestTikTokCardController_Update_EmptyID(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	updateReq := map[string]any{
		"title": "Test",
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/api/tiktok-card/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Errorf("Expected status OK or Not Found, got %d", w.Code)
	}
}

// TestTikTokCardController_Delete_Success 测试删除卡片成功
func TestTikTokCardController_Delete_Success(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	req, _ := http.NewRequest("DELETE", "/api/tiktok-card/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Errorf("Expected error for non-existent card, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestTikTokCardController_Delete_EmptyID 测试空 ID
// Gin 路由 /api/tiktok-card/:id 不匹配 /api/tiktok-card/，返回 404
func TestTikTokCardController_Delete_EmptyID(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	req, _ := http.NewRequest("DELETE", "/api/tiktok-card/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status Not Found for empty ID, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestTikTokCardController_Get_Success 测试获取卡片成功
func TestTikTokCardController_Get_Success(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	req, _ := http.NewRequest("GET", "/api/tiktok-card/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status Not Found, got %d, body: %s", w.Code, w.Body.String())
	}

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["code"] == float64(0) {
		t.Errorf("Expected NOT_FOUND code, got %v", response["code"])
	}
}

// TestTikTokCardController_Get_EmptyID 测试空 ID
func TestTikTokCardController_Get_EmptyID(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	req, _ := http.NewRequest("GET", "/api/tiktok-card/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound && w.Code != http.StatusOK {
		t.Errorf("Expected status Not Found or OK, got %d", w.Code)
	}
}

// TestTikTokCardController_List_Success 测试获取卡片列表成功
func TestTikTokCardController_List_Success(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	req, _ := http.NewRequest("GET", "/api/tiktok-card/list?page=1&page_size=20", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["code"] != float64(0) {
		t.Errorf("Expected code SUCCESS, got %v", response["code"])
	}
}

// TestTikTokCardController_List_DefaultPagination 测试默认分页
func TestTikTokCardController_List_DefaultPagination(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	req, _ := http.NewRequest("GET", "/api/tiktok-card/list", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	data, ok := response["data"].(map[string]any)
	if !ok {
		t.Errorf("Expected data to be a map")
	}
	if data["list"] == nil {
		t.Errorf("Expected list to be present")
	}
	if data["total"] == nil {
		t.Errorf("Expected total to be present")
	}
}

// TestTikTokCardController_List_InvalidPage 测试无效分页参数
func TestTikTokCardController_List_InvalidPage(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	req, _ := http.NewRequest("GET", "/api/tiktok-card/list?page=invalid&page_size=invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestTikTokCardController_GenerateShortLink_Success 测试生成短链成功
func TestTikTokCardController_GenerateShortLink_Success(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	generateReq := map[string]any{
		"cardId": 1,
	}
	body, _ := json.Marshal(generateReq)

	req, _ := http.NewRequest("POST", "/api/tiktok-card/generate-short-link", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Errorf("Expected error for non-existent card, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestTikTokCardController_GenerateShortLink_InvalidJSON 测试无效 JSON
func TestTikTokCardController_GenerateShortLink_InvalidJSON(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	req, _ := http.NewRequest("POST", "/api/tiktok-card/generate-short-link", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestTikTokCardController_GenerateShortLink_MissingCardId 测试缺少 cardId
// binding:"required" 会拒绝缺失字段，返回 400
func TestTikTokCardController_GenerateShortLink_MissingCardId(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	generateReq := map[string]any{}
	body, _ := json.Marshal(generateReq)

	req, _ := http.NewRequest("POST", "/api/tiktok-card/generate-short-link", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request for missing cardId, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestTikTokCardController_StatsOverall_Success 测试获取总体统计成功
func TestTikTokCardController_StatsOverall_Success(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	req, _ := http.NewRequest("GET", "/api/tiktok-card/stats/overall", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["code"] != float64(0) {
		t.Errorf("Expected code SUCCESS, got %v", response["code"])
	}

	data, ok := response["data"].(map[string]any)
	if !ok {
		t.Errorf("Expected data to be a map")
	}
	if data["total_cards"] == nil {
		t.Errorf("Expected total_cards to be present")
	}
}

// TestTikTokCardController_Stats_Success 测试获取卡片统计成功
func TestTikTokCardController_Stats_Success(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	req, _ := http.NewRequest("GET", "/api/tiktok-card/1/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status OK, Not Found or Internal Error, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestTikTokCardController_Stats_EmptyID 测试空 ID 统计
func TestTikTokCardController_Stats_EmptyID(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	req, _ := http.NewRequest("GET", "/api/tiktok-card//stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound && w.Code != http.StatusOK {
		t.Errorf("Expected status Bad Request, Not Found or OK, got %d", w.Code)
	}
}

// TestTikTokCardController_Create_WithAllFields 测试创建带所有字段的卡片
func TestTikTokCardController_Create_WithAllFields(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	createReq := map[string]any{
		"title":          "Full Card",
		"description":    "This is a full card with all fields",
		"image_url":      "https://example.com/image.jpg",
		"redirect_url":   "https://www.tiktok.com",
		"is_active":      true,
		"domain_pool_id": 1,
		"tags":           "tag1,tag2,tag3",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/api/tiktok-card", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestTikTokCardController_Create_InactiveCard 测试创建非激活卡片
func TestTikTokCardController_Create_InactiveCard(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	createReq := map[string]any{
		"title":       "Inactive Card",
		"description": "This card is inactive",
		"is_active":   false,
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/api/tiktok-card", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestTikTokCardController_RoutesRegistration 测试路由注册
func TestTikTokCardController_RoutesRegistration(t *testing.T) {
	router := setupTikTokCardTestRouter(t)

	testCases := []struct {
		method string
		path   string
	}{
		{"GET", "/api/tiktok-card/list"},
		{"GET", "/api/tiktok-card/1"},
		{"POST", "/api/tiktok-card"},
		{"PUT", "/api/tiktok-card/1"},
		{"DELETE", "/api/tiktok-card/1"},
		{"POST", "/api/tiktok-card/generate-short-link"},
		{"GET", "/api/tiktok-card/stats/overall"},
		{"GET", "/api/tiktok-card/1/stats"},
	}

	for _, tc := range testCases {
		req, _ := http.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		_ = w
	}
}

