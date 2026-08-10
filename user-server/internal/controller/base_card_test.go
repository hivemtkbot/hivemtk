package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/service"

	"hivemtk-user/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupTestDouyinCardService 设置测试服务
func setupTestDouyinCardService(t *testing.T) (service.DouyinCardService, *gorm.DB) {
	database := testutil.NewTestDB(t,
		&model.DouyinCard{},
		&model.ShortLink{},
		&model.DomainPool{},
	)
	db.SetTestDB(database)
	return service.NewDouyinCardService(database), database
}

// TestBaseCardController_Create_Success 测试创建卡片成功（通过实际的 DouyinCardController）
func TestBaseCardController_Create_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _ := setupTestDouyinCardService(t)
	ctrl := NewDouyinCardController(svc)
	router := gin.New()

	router.POST("/cards", ctrl.Create)

	createReq := dto.DouyinCardCreateRequest{
		Title:        "Test Card",
		Description:  "This is a test card",
		ImageURL:     "https://example.com/image.jpg",
		RedirectURL:  "https://www.douyin.com",
		DomainPoolID: 1,
		Tags:         "test,card",
		IsActive:     true,
	}

	w := httptest.NewRecorder()
	body := createReqToJSON(t, createReq)
	req := httptest.NewRequest("POST", "/cards", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestBaseCardController_Create_InvalidJSON 测试无效 JSON 请求
func TestBaseCardController_Create_InvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _ := setupTestDouyinCardService(t)
	ctrl := NewDouyinCardController(svc)
	router := gin.New()

	router.POST("/cards", ctrl.Create)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/cards", strings.NewReader("invalid-json"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestBaseCardController_Update_Success 测试更新卡片成功
func TestBaseCardController_Update_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, db := setupTestDouyinCardService(t)
	ctrl := NewDouyinCardController(svc)
	router := gin.New()

	// 先创建卡片
	card := &model.DouyinCard{
		Title:        "Original Card",
		Description:  "Original description",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: 1,
		IsActive:     true,
	}
	db.Create(card)

	router.PUT("/cards/:id", ctrl.Update)

	updateReq := dto.DouyinCardUpdateRequest{
		ID:           card.ID,
		Title:        "Updated Card",
		Description:  "Updated description",
		ImageURL:     "https://example.com/new-image.jpg",
		RedirectURL:  "https://www.douyin.com/new",
		DomainPoolID: 1,
		Tags:         "updated,card",
		ViewCount:    100,
		IsActive:     true,
	}

	w := httptest.NewRecorder()
	body := createReqToJSON(t, updateReq)
	req := httptest.NewRequest("PUT", "/cards/1", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestBaseCardController_Update_InvalidJSON 测试无效 JSON
func TestBaseCardController_Update_InvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _ := setupTestDouyinCardService(t)
	ctrl := NewDouyinCardController(svc)
	router := gin.New()

	router.PUT("/cards/:id", ctrl.Update)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/cards/1", strings.NewReader("invalid-json"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestBaseCardController_Delete_Success 测试删除卡片成功
func TestBaseCardController_Delete_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, db := setupTestDouyinCardService(t)
	ctrl := NewDouyinCardController(svc)
	router := gin.New()

	// 先创建卡片
	card := &model.DouyinCard{
		Title:        "Card to Delete",
		Description:  "This card will be deleted",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: 1,
		IsActive:     true,
	}
	db.Create(card)

	router.DELETE("/cards/:id", ctrl.Delete)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/cards/1", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestBaseCardController_Delete_InvalidID 测试无效 ID
func TestBaseCardController_Delete_InvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _ := setupTestDouyinCardService(t)
	ctrl := NewDouyinCardController(svc)
	router := gin.New()

	router.DELETE("/cards/:id", ctrl.Delete)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/cards/invalid", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestBaseCardController_GetByID_Success 测试根据 ID 获取卡片成功
func TestBaseCardController_GetByID_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, db := setupTestDouyinCardService(t)
	ctrl := NewDouyinCardController(svc)
	router := gin.New()

	// 先创建卡片
	card := &model.DouyinCard{
		Title:        "Test Card",
		Description:  "Test description",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: 1,
		IsActive:     true,
	}
	db.Create(card)

	router.GET("/cards/:id", ctrl.GetByID)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/cards/1", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestBaseCardController_GetByID_NotFound 测试卡片不存在
func TestBaseCardController_GetByID_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _ := setupTestDouyinCardService(t)
	ctrl := NewDouyinCardController(svc)
	router := gin.New()

	router.GET("/cards/:id", ctrl.GetByID)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/cards/999", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status Not Found, got %d", w.Code)
	}
}

// TestBaseCardController_GetByID_InvalidID 测试无效 ID 格式
func TestBaseCardController_GetByID_InvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _ := setupTestDouyinCardService(t)
	ctrl := NewDouyinCardController(svc)
	router := gin.New()

	router.GET("/cards/:id", ctrl.GetByID)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/cards/invalid", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestBaseCardController_GetList_Success 测试获取卡片列表成功
func TestBaseCardController_GetList_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, db := setupTestDouyinCardService(t)
	ctrl := NewDouyinCardController(svc)
	router := gin.New()

	// 创建多个卡片
	for i := 1; i <= 5; i++ {
		db.Create(&model.DouyinCard{
			Title:        "Card " + strconv.Itoa(i),
			Description:  "Description " + strconv.Itoa(i),
			ImageURL:     "https://example.com/image" + strconv.Itoa(i) + ".jpg",
			DomainPoolID: 1,
			IsActive:     true,
		})
	}

	router.GET("/cards", ctrl.GetList)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/cards?page=1&page_size=10", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestBaseCardController_GetList_DefaultPagination 测试默认分页
func TestBaseCardController_GetList_DefaultPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, db := setupTestDouyinCardService(t)
	ctrl := NewDouyinCardController(svc)
	router := gin.New()

	// 创建测试卡片
	db.Create(&model.DouyinCard{
		Title:        "Card",
		Description:  "Description",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: 1,
		IsActive:     true,
	})

	router.GET("/cards", ctrl.GetList)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/cards", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestParseIDParam 测试 parseIDParam 函数
func TestParseIDParam(t *testing.T) {
	tests := []struct {
		name    string
		paramID string
		wantID  uint
		wantErr bool
	}{
		{"valid_id", "123", 123, false},
		{"valid_id_zero", "0", 0, false},
		{"invalid_id_string", "abc", 0, true},
		{"invalid_id_negative", "-1", 0, true},
		{"invalid_id_float", "1.5", 0, true},
		{"invalid_id_empty", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.GET("/test/:id", func(ctx *gin.Context) {
				id, err := parseIDParam(ctx)
				if (err != nil) != tt.wantErr {
					t.Errorf("parseIDParam() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if id != tt.wantID {
					t.Errorf("parseIDParam() = %v, want %v", id, tt.wantID)
				}
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/test/"+tt.paramID, nil)
			router.ServeHTTP(w, req)
		})
	}
}

// TestParseIDWithValidation 测试 ParseIDWithValidation 函数
func TestParseIDWithValidation(t *testing.T) {
	tests := []struct {
		name      string
		paramID   string
		reqID     uint
		wantID    uint
		wantMatch bool
		wantErr   bool
	}{
		{"valid_id_match", "123", 123, 123, true, false},
		{"valid_id_mismatch", "123", 456, 123, false, false},
		{"req_id_zero_use_uri", "123", 0, 123, true, false},
		{"invalid_param_id", "abc", 123, 0, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.GET("/test/:id", func(ctx *gin.Context) {
				id, matched, err := ParseIDWithValidation(ctx, tt.reqID)
				if (err != nil) != tt.wantErr {
					t.Errorf("ParseIDWithValidation() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if id != tt.wantID {
					t.Errorf("ParseIDWithValidation() id = %v, want %v", id, tt.wantID)
				}
				if matched != tt.wantMatch {
					t.Errorf("ParseIDWithValidation() matched = %v, want %v", matched, tt.wantMatch)
				}
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/test/"+tt.paramID, nil)
			router.ServeHTTP(w, req)
		})
	}
}

// 辅助函数
func createReqToJSON(t *testing.T, req any) *strings.Reader {
	t.Helper()
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}
	return strings.NewReader(string(data))
}
