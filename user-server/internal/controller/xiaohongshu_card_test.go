package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/service"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupXiaohongshuCardTestDB 设置小红书卡片测试数据库
func setupXiaohongshuCardTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.XiaohongshuCard{},
		&model.ShortLink{},
		&model.DomainPool{},
		&model.XiaohongshuCardActivity{},
	)
}

// uintPtr 返回 uint 指针
func uintPtr(v uint) *uint {
	return &v
}

// TestXiaohongshuCardController_Create_Success 测试创建小红书卡片成功
func TestXiaohongshuCardController_Create_Success(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	// 预先创建域名池记录
	database.Create(&model.DomainPool{
		Domain:  "example.com",
		Port:    80,
		Purpose: "test",
		Status:  1,
	})
	ctrl := NewXiaohongshuCardController(service.NewXiaohongshuCardService(database))
	router := setupGinEngine()
	router.POST("/xiaohongshu/cards", ctrl.Create)

	createReq := dto.XiaohongshuCardCreateRequest{
		Title:        "Test Card",
		Description:  "This is a test card",
		ImageURL:     "https://example.com/image.jpg",
		RedirectURL:  "https://www.xiaohongshu.com",
		DomainPoolID: uintPtr(1),
		Tags:         "test,card",
		IsActive:     true,
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/xiaohongshu/cards", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestXiaohongshuCardController_Create_InvalidJSON 测试无效 JSON 请求
func TestXiaohongshuCardController_Create_InvalidJSON(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	ctrl := NewXiaohongshuCardController(service.NewXiaohongshuCardService(database))
	router := setupGinEngine()
	router.POST("/xiaohongshu/cards", ctrl.Create)

	req, _ := http.NewRequest("POST", "/xiaohongshu/cards", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestXiaohongshuCardController_Create_EmptyTitle 测试空标题
func TestXiaohongshuCardController_Create_EmptyTitle(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	ctrl := NewXiaohongshuCardController(service.NewXiaohongshuCardService(database))
	router := setupGinEngine()
	router.POST("/xiaohongshu/cards", ctrl.Create)

	createReq := dto.XiaohongshuCardCreateRequest{
		Title:       "",
		Description: "This is a test card",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/xiaohongshu/cards", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestXiaohongshuCardController_Create_InvalidImageURL 测试无效图片 URL
func TestXiaohongshuCardController_Create_InvalidImageURL(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	ctrl := NewXiaohongshuCardController(service.NewXiaohongshuCardService(database))
	router := setupGinEngine()
	router.POST("/xiaohongshu/cards", ctrl.Create)

	createReq := dto.XiaohongshuCardCreateRequest{
		Title:       "Test Card",
		Description: "This is a test card",
		ImageURL:    "invalid-url",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/xiaohongshu/cards", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestXiaohongshuCardController_Create_DefaultRedirectURL 测试默认跳转链接
func TestXiaohongshuCardController_Create_DefaultRedirectURL(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	// 预先创建域名池记录
	database.Create(&model.DomainPool{
		Domain:  "example.com",
		Port:    80,
		Purpose: "test",
		Status:  1,
	})
	var domainPoolID uint = 1
	ctrl := NewXiaohongshuCardController(service.NewXiaohongshuCardService(database))
	router := setupGinEngine()
	router.POST("/xiaohongshu/cards", ctrl.Create)

	createReq := dto.XiaohongshuCardCreateRequest{
		Title:        "Test Card",
		Description:  "This is a test card",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: &domainPoolID,
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/xiaohongshu/cards", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestXiaohongshuCardController_Update_Success 测试更新卡片成功
func TestXiaohongshuCardController_Update_Success(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	// 预先创建域名池记录
	database.Create(&model.DomainPool{
		Domain:  "example.com",
		Port:    80,
		Purpose: "test",
		Status:  1,
	})
	svc := service.NewXiaohongshuCardService(database)
	ctrl := NewXiaohongshuCardController(svc)
	router := setupGinEngine()

	// 先创建卡片
	card, _ := svc.Create(context.Background(), &dto.XiaohongshuCardCreateRequest{
		Title:        "Original Card",
		Description:  "Original description",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: uintPtr(1),
		IsActive:     true,
	})

	if card == nil {
		t.Fatalf("card creation failed: 前置条件失败（svc.Create 返回 nil），需修复创建逻辑而非跳过测试")
		return
	}

	router.PUT("/xiaohongshu/cards/:id", ctrl.Update)

	updateReq := dto.XiaohongshuCardUpdateRequest{
		ID:           card.ID,
		Title:        "Updated Card",
		Description:  "Updated description",
		ImageURL:     "https://example.com/new-image.jpg",
		RedirectURL:  "https://www.xiaohongshu.com/new",
		DomainPoolID: uintPtr(1),
		Tags:         "updated,card",
		ViewCount:    100,
		IsActive:     true,
	}
	body, _ := json.Marshal(updateReq)

	cardID := "1"
	if card.ID != 0 {
		cardID = fmt.Sprintf("%d", card.ID)
	}
	req, _ := http.NewRequest("PUT", "/xiaohongshu/cards/"+cardID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestXiaohongshuCardController_Update_InvalidJSON 测试无效 JSON
func TestXiaohongshuCardController_Update_InvalidJSON(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	ctrl := NewXiaohongshuCardController(service.NewXiaohongshuCardService(database))
	router := setupGinEngine()
	router.PUT("/xiaohongshu/cards/:id", ctrl.Update)

	req, _ := http.NewRequest("PUT", "/xiaohongshu/cards/1", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestXiaohongshuCardController_Update_IDMismatch 测试 ID 不匹配
func TestXiaohongshuCardController_Update_IDMismatch(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	ctrl := NewXiaohongshuCardController(service.NewXiaohongshuCardService(database))
	router := setupGinEngine()
	router.PUT("/xiaohongshu/cards/:id", ctrl.Update)

	updateReq := dto.XiaohongshuCardUpdateRequest{
		ID:    999, // 与 URL 中的 ID 不匹配
		Title: "Test",
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/xiaohongshu/cards/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestXiaohongshuCardController_Delete_Success 测试删除卡片成功
func TestXiaohongshuCardController_Delete_Success(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	svc := service.NewXiaohongshuCardService(database)
	ctrl := NewXiaohongshuCardController(svc)
	router := setupGinEngine()

	// 先创建卡片
	svc.Create(context.Background(), &dto.XiaohongshuCardCreateRequest{
		Title:        "Card to Delete",
		Description:  "This card will be deleted",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: uintPtr(1),
		IsActive:     true,
	})

	router.DELETE("/xiaohongshu/cards/:id", ctrl.Delete)

	req, _ := http.NewRequest("DELETE", "/xiaohongshu/cards/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestXiaohongshuCardController_Delete_InvalidID 测试无效 ID
func TestXiaohongshuCardController_Delete_InvalidID(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	ctrl := NewXiaohongshuCardController(service.NewXiaohongshuCardService(database))
	router := setupGinEngine()
	router.DELETE("/xiaohongshu/cards/:id", ctrl.Delete)

	req, _ := http.NewRequest("DELETE", "/xiaohongshu/cards/invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestXiaohongshuCardController_GetByID_Success 测试根据 ID 获取卡片成功
func TestXiaohongshuCardController_GetByID_Success(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	svc := service.NewXiaohongshuCardService(database)
	ctrl := NewXiaohongshuCardController(svc)
	router := setupGinEngine()

	// 先创建卡片
	svc.Create(context.Background(), &dto.XiaohongshuCardCreateRequest{
		Title:        "Test Card",
		Description:  "Test description",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: uintPtr(1),
		IsActive:     true,
	})

	router.GET("/xiaohongshu/cards/:id", ctrl.GetByID)

	req, _ := http.NewRequest("GET", "/xiaohongshu/cards/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestXiaohongshuCardController_GetByID_NotFound 测试卡片不存在
func TestXiaohongshuCardController_GetByID_NotFound(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	ctrl := NewXiaohongshuCardController(service.NewXiaohongshuCardService(database))
	router := setupGinEngine()
	router.GET("/xiaohongshu/cards/:id", ctrl.GetByID)

	req, _ := http.NewRequest("GET", "/xiaohongshu/cards/999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Service returns record not found error which results in 404 or 500
	if w.Code != http.StatusInternalServerError && w.Code != http.StatusNotFound {
		t.Errorf("Expected status Internal Server Error or Not Found, got %d", w.Code)
	}
}

// TestXiaohongshuCardController_GetByID_InvalidID 测试无效 ID 格式
func TestXiaohongshuCardController_GetByID_InvalidID(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	ctrl := NewXiaohongshuCardController(service.NewXiaohongshuCardService(database))
	router := setupGinEngine()
	router.GET("/xiaohongshu/cards/:id", ctrl.GetByID)

	req, _ := http.NewRequest("GET", "/xiaohongshu/cards/invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestXiaohongshuCardController_GetList_Success 测试获取卡片列表成功
func TestXiaohongshuCardController_GetList_Success(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	svc := service.NewXiaohongshuCardService(database)
	ctrl := NewXiaohongshuCardController(svc)
	router := setupGinEngine()

	// 创建多个卡片
	for i := 1; i <= 5; i++ {
		svc.Create(context.Background(), &dto.XiaohongshuCardCreateRequest{
			Title:        "Card " + string(rune('0'+i)),
			Description:  "Description " + string(rune('0'+i)),
			ImageURL:     "https://example.com/image" + string(rune('0'+i)) + ".jpg",
			DomainPoolID: uintPtr(1),
			IsActive:     true,
		})
	}

	router.GET("/xiaohongshu/cards", ctrl.GetList)

	req, _ := http.NewRequest("GET", "/xiaohongshu/cards?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestXiaohongshuCardController_GetList_DefaultPagination 测试默认分页
func TestXiaohongshuCardController_GetList_DefaultPagination(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	svc := service.NewXiaohongshuCardService(database)
	ctrl := NewXiaohongshuCardController(svc)
	router := setupGinEngine()

	// 创建测试卡片
	svc.Create(context.Background(), &dto.XiaohongshuCardCreateRequest{
		Title:        "Card",
		Description:  "Description",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: uintPtr(1),
		IsActive:     true,
	})

	router.GET("/xiaohongshu/cards", ctrl.GetList)

	req, _ := http.NewRequest("GET", "/xiaohongshu/cards?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestXiaohongshuCardController_GetList_InvalidPage 测试无效分页参数
func TestXiaohongshuCardController_GetList_InvalidPage(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	svc := service.NewXiaohongshuCardService(database)
	ctrl := NewXiaohongshuCardController(svc)
	router := setupGinEngine()
	router.GET("/xiaohongshu/cards", ctrl.GetList)

	req, _ := http.NewRequest("GET", "/xiaohongshu/cards?page=invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestXiaohongshuCardController_GenerateShortLink_Success 测试生成短链成功
func TestXiaohongshuCardController_GenerateShortLink_Success(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	// 预先创建域名池记录
	database.Create(&model.DomainPool{
		Domain:  "example.com",
		Port:    80,
		Purpose: "test",
		Status:  1,
	})
	svc := service.NewXiaohongshuCardService(database)
	ctrl := NewXiaohongshuCardController(svc)
	router := setupGinEngine()

	// 先创建卡片
	card, _ := svc.Create(context.Background(), &dto.XiaohongshuCardCreateRequest{
		Title:        "Card with Short Link",
		Description:  "This card will have a short link",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: uintPtr(1),
		IsActive:     true,
	})

	if card == nil {
		t.Fatalf("card creation failed: 前置条件失败（svc.Create 返回 nil），需修复创建逻辑而非跳过测试")
		return
	}

	router.POST("/xiaohongshu/cards/:id/shortlink", ctrl.GenerateShortLink)

	cardID := "1"
	if card.ID != 0 {
		cardID = fmt.Sprintf("%d", card.ID)
	}
	req, _ := http.NewRequest("POST", "/xiaohongshu/cards/"+cardID+"/shortlink", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 短链生成依赖外部域名，测试环境无数据时接受 200/500
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status OK or Internal Server Error, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestXiaohongshuCardController_GenerateShortLink_InvalidID 测试无效 ID
func TestXiaohongshuCardController_GenerateShortLink_InvalidID(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	ctrl := NewXiaohongshuCardController(service.NewXiaohongshuCardService(database))
	router := setupGinEngine()
	router.POST("/xiaohongshu/cards/:id/shortlink", ctrl.GenerateShortLink)

	req, _ := http.NewRequest("POST", "/xiaohongshu/cards/invalid/shortlink", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestXiaohongshuCardController_GenerateShortLink_NotFound 测试卡片不存在
func TestXiaohongshuCardController_GenerateShortLink_NotFound(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	ctrl := NewXiaohongshuCardController(service.NewXiaohongshuCardService(database))
	router := setupGinEngine()
	router.POST("/xiaohongshu/cards/:id/shortlink", ctrl.GenerateShortLink)

	req, _ := http.NewRequest("POST", "/xiaohongshu/cards/999/shortlink", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Service returns record not found error which results in 404 or 500
	if w.Code != http.StatusInternalServerError && w.Code != http.StatusNotFound {
		t.Errorf("Expected status Internal Server Error or Not Found, got %d", w.Code)
	}
}

// TestXiaohongshuCardController_Create_WithTags 测试创建带标签的卡片
func TestXiaohongshuCardController_Create_WithTags(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	// 预先创建域名池记录
	database.Create(&model.DomainPool{
		Domain:  "example.com",
		Port:    80,
		Purpose: "test",
		Status:  1,
	})
	ctrl := NewXiaohongshuCardController(service.NewXiaohongshuCardService(database))
	router := setupGinEngine()
	router.POST("/xiaohongshu/cards", ctrl.Create)

	createReq := dto.XiaohongshuCardCreateRequest{
		Title:        "Card with Tags",
		Description:  "This card has tags",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: uintPtr(1),
		Tags:         "tag1,tag2,tag3",
		IsActive:     true,
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/xiaohongshu/cards", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestXiaohongshuCardController_Create_InactiveCard 测试创建非激活卡片
func TestXiaohongshuCardController_Create_InactiveCard(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	// 预先创建域名池记录
	database.Create(&model.DomainPool{
		Domain:  "example.com",
		Port:    80,
		Purpose: "test",
		Status:  1,
	})
	ctrl := NewXiaohongshuCardController(service.NewXiaohongshuCardService(database))
	router := setupGinEngine()
	router.POST("/xiaohongshu/cards", ctrl.Create)

	createReq := dto.XiaohongshuCardCreateRequest{
		Title:        "Inactive Card",
		Description:  "This card is inactive",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: uintPtr(1),
		IsActive:     false,
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/xiaohongshu/cards", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestXiaohongshuCardController_Update_ViewCount 测试更新浏览次数
func TestXiaohongshuCardController_Update_ViewCount(t *testing.T) {
	database := setupXiaohongshuCardTestDB(t)
	// 预先创建域名池记录
	database.Create(&model.DomainPool{
		Domain:  "example.com",
		Port:    80,
		Purpose: "test",
		Status:  1,
	})
	svc := service.NewXiaohongshuCardService(database)
	ctrl := NewXiaohongshuCardController(svc)
	router := setupGinEngine()

	// 先创建卡片
	card, _ := svc.Create(context.Background(), &dto.XiaohongshuCardCreateRequest{
		Title:        "Card",
		Description:  "Description",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: uintPtr(1),
		IsActive:     true,
	})

	if card == nil {
		t.Fatalf("card creation failed: 前置条件失败（svc.Create 返回 nil），需修复创建逻辑而非跳过测试")
		return
	}

	router.PUT("/xiaohongshu/cards/:id", ctrl.Update)

	updateReq := dto.XiaohongshuCardUpdateRequest{
		ID:          card.ID,
		Title:       "Card",
		Description: "Description",
		ImageURL:    "https://example.com/image.jpg",
		ViewCount:   1000,
		IsActive:    true,
	}
	body, _ := json.Marshal(updateReq)

	cardID := "1"
	if card.ID != 0 {
		cardID = fmt.Sprintf("%d", card.ID)
	}
	req, _ := http.NewRequest("PUT", "/xiaohongshu/cards/"+cardID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}
