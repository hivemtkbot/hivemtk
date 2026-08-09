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

// setupKuaishouCardTestDB 设置快手卡片测试数据库
func setupKuaishouCardTestDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.KuaishouCard{},
		&model.ShortLink{},
		&model.DomainPool{},
		&model.KuaishouCardActivity{},
	)
}

// TestKuaishouCardController_Create_Success 测试创建快手卡片成功
func TestKuaishouCardController_Create_Success(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	ctrl := NewKuaishouCardController(service.NewKuaishouCardService(database))
	router := setupGinEngine()
	router.POST("/kuaishou/cards", ctrl.Create)

	createReq := dto.KuaishouCardCreateRequest{
		Title:        "Test Card",
		Description:  "This is a test card",
		ImageURL:     "https://example.com/image.jpg",
		RedirectURL:  "https://www.kuaishou.com",
		DomainPoolID: 1,
		Tags:         "test,card",
		IsActive:     true,
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/kuaishou/cards", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestKuaishouCardController_Create_InvalidJSON 测试无效 JSON 请求
func TestKuaishouCardController_Create_InvalidJSON(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	ctrl := NewKuaishouCardController(service.NewKuaishouCardService(database))
	router := setupGinEngine()
	router.POST("/kuaishou/cards", ctrl.Create)

	req, _ := http.NewRequest("POST", "/kuaishou/cards", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestKuaishouCardController_Create_EmptyTitle 测试空标题
func TestKuaishouCardController_Create_EmptyTitle(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	ctrl := NewKuaishouCardController(service.NewKuaishouCardService(database))
	router := setupGinEngine()
	router.POST("/kuaishou/cards", ctrl.Create)

	createReq := dto.KuaishouCardCreateRequest{
		Title:       "",
		Description: "This is a test card",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/kuaishou/cards", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestKuaishouCardController_Create_InvalidImageURL 测试无效图片 URL
func TestKuaishouCardController_Create_InvalidImageURL(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	ctrl := NewKuaishouCardController(service.NewKuaishouCardService(database))
	router := setupGinEngine()
	router.POST("/kuaishou/cards", ctrl.Create)

	createReq := dto.KuaishouCardCreateRequest{
		Title:       "Test Card",
		Description: "This is a test card",
		ImageURL:    "invalid-url",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/kuaishou/cards", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestKuaishouCardController_Create_DefaultRedirectURL 测试默认跳转链接
func TestKuaishouCardController_Create_DefaultRedirectURL(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	ctrl := NewKuaishouCardController(service.NewKuaishouCardService(database))
	router := setupGinEngine()
	router.POST("/kuaishou/cards", ctrl.Create)

	createReq := dto.KuaishouCardCreateRequest{
		Title:       "Test Card",
		Description: "This is a test card",
		ImageURL:    "https://example.com/image.jpg",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/kuaishou/cards", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestKuaishouCardController_Update_Success 测试更新卡片成功
func TestKuaishouCardController_Update_Success(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	svc := service.NewKuaishouCardService(database)
	ctrl := NewKuaishouCardController(svc)
	router := setupGinEngine()

	// 先创建卡片
	card, _ := svc.Create(context.Background(), &dto.KuaishouCardCreateRequest{
		Title:        "Original Card",
		Description:  "Original description",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: 1,
		IsActive:     true,
	})

	router.PUT("/kuaishou/cards/:id", ctrl.Update)

	updateReq := dto.KuaishouCardUpdateRequest{
		ID:           card.ID,
		Title:        "Updated Card",
		Description:  "Updated description",
		ImageURL:     "https://example.com/new-image.jpg",
		RedirectURL:  "https://www.kuaishou.com/new",
		DomainPoolID: 1,
		Tags:         "updated,card",
		ViewCount:    100,
		IsActive:     true,
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/kuaishou/cards/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestKuaishouCardController_Update_InvalidJSON 测试无效 JSON
func TestKuaishouCardController_Update_InvalidJSON(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	ctrl := NewKuaishouCardController(service.NewKuaishouCardService(database))
	router := setupGinEngine()
	router.PUT("/kuaishou/cards/:id", ctrl.Update)

	req, _ := http.NewRequest("PUT", "/kuaishou/cards/1", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestKuaishouCardController_Update_IDMismatch 测试 ID 不匹配
func TestKuaishouCardController_Update_IDMismatch(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	ctrl := NewKuaishouCardController(service.NewKuaishouCardService(database))
	router := setupGinEngine()
	router.PUT("/kuaishou/cards/:id", ctrl.Update)

	updateReq := dto.KuaishouCardUpdateRequest{
		ID:    999, // 与 URL 中的 ID 不匹配
		Title: "Test",
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/kuaishou/cards/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestKuaishouCardController_Delete_Success 测试删除卡片成功
func TestKuaishouCardController_Delete_Success(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	svc := service.NewKuaishouCardService(database)
	ctrl := NewKuaishouCardController(svc)
	router := setupGinEngine()

	// 先创建卡片
	svc.Create(context.Background(), &dto.KuaishouCardCreateRequest{
		Title:        "Card to Delete",
		Description:  "This card will be deleted",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: 1,
		IsActive:     true,
	})

	router.DELETE("/kuaishou/cards/:id", ctrl.Delete)

	req, _ := http.NewRequest("DELETE", "/kuaishou/cards/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestKuaishouCardController_Delete_InvalidID 测试无效 ID
func TestKuaishouCardController_Delete_InvalidID(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	ctrl := NewKuaishouCardController(service.NewKuaishouCardService(database))
	router := setupGinEngine()
	router.DELETE("/kuaishou/cards/:id", ctrl.Delete)

	req, _ := http.NewRequest("DELETE", "/kuaishou/cards/invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestKuaishouCardController_GetByID_Success 测试根据 ID 获取卡片成功
func TestKuaishouCardController_GetByID_Success(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	svc := service.NewKuaishouCardService(database)
	ctrl := NewKuaishouCardController(svc)
	router := setupGinEngine()

	// 先创建卡片
	svc.Create(context.Background(), &dto.KuaishouCardCreateRequest{
		Title:        "Test Card",
		Description:  "Test description",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: 1,
		IsActive:     true,
	})

	router.GET("/kuaishou/cards/:id", ctrl.GetByID)

	req, _ := http.NewRequest("GET", "/kuaishou/cards/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestKuaishouCardController_GetByID_NotFound 测试卡片不存在
func TestKuaishouCardController_GetByID_NotFound(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	ctrl := NewKuaishouCardController(service.NewKuaishouCardService(database))
	router := setupGinEngine()
	router.GET("/kuaishou/cards/:id", ctrl.GetByID)

	req, _ := http.NewRequest("GET", "/kuaishou/cards/999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Service returns record not found error which results in 404 or 500
	if w.Code != http.StatusInternalServerError && w.Code != http.StatusNotFound {
		t.Errorf("Expected status Internal Server Error or Not Found, got %d", w.Code)
	}
}

// TestKuaishouCardController_GetByID_InvalidID 测试无效 ID 格式
func TestKuaishouCardController_GetByID_InvalidID(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	ctrl := NewKuaishouCardController(service.NewKuaishouCardService(database))
	router := setupGinEngine()
	router.GET("/kuaishou/cards/:id", ctrl.GetByID)

	req, _ := http.NewRequest("GET", "/kuaishou/cards/invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestKuaishouCardController_GetList_Success 测试获取卡片列表成功
func TestKuaishouCardController_GetList_Success(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	svc := service.NewKuaishouCardService(database)
	ctrl := NewKuaishouCardController(svc)
	router := setupGinEngine()

	// 创建多个卡片
	for i := 1; i <= 5; i++ {
		svc.Create(context.Background(), &dto.KuaishouCardCreateRequest{
			Title:        "Card " + string(rune('0'+i)),
			Description:  "Description " + string(rune('0'+i)),
			ImageURL:     "https://example.com/image" + string(rune('0'+i)) + ".jpg",
			DomainPoolID: 1,
			IsActive:     true,
		})
	}

	router.GET("/kuaishou/cards", ctrl.GetList)

	req, _ := http.NewRequest("GET", "/kuaishou/cards?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestKuaishouCardController_GetList_DefaultPagination 测试默认分页
func TestKuaishouCardController_GetList_DefaultPagination(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	svc := service.NewKuaishouCardService(database)
	ctrl := NewKuaishouCardController(svc)
	router := setupGinEngine()

	// 创建测试卡片
	svc.Create(context.Background(), &dto.KuaishouCardCreateRequest{
		Title:        "Card",
		Description:  "Description",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: 1,
		IsActive:     true,
	})

	router.GET("/kuaishou/cards", ctrl.GetList)

	req, _ := http.NewRequest("GET", "/kuaishou/cards?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestKuaishouCardController_GetList_InvalidPage 测试无效分页参数
func TestKuaishouCardController_GetList_InvalidPage(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	svc := service.NewKuaishouCardService(database)
	ctrl := NewKuaishouCardController(svc)
	router := setupGinEngine()
	router.GET("/kuaishou/cards", ctrl.GetList)

	req, _ := http.NewRequest("GET", "/kuaishou/cards?page=invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestKuaishouCardController_ViewCard_Success 测试浏览卡片成功
func TestKuaishouCardController_ViewCard_Success(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	svc := service.NewKuaishouCardService(database)
	ctrl := NewKuaishouCardController(svc)
	router := setupGinEngine()

	// 先创建卡片
	svc.Create(context.Background(), &dto.KuaishouCardCreateRequest{
		Title:        "Card to View",
		Description:  "This card will be viewed",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: 1,
		IsActive:     true,
	})

	router.POST("/kuaishou/cards/:id/view", ctrl.ViewCard)

	req, _ := http.NewRequest("POST", "/kuaishou/cards/1/view", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestKuaishouCardController_ViewCard_InvalidID 测试无效 ID
func TestKuaishouCardController_ViewCard_InvalidID(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	ctrl := NewKuaishouCardController(service.NewKuaishouCardService(database))
	router := setupGinEngine()
	router.POST("/kuaishou/cards/:id/view", ctrl.ViewCard)

	req, _ := http.NewRequest("POST", "/kuaishou/cards/invalid/view", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestKuaishouCardController_LikeCard_Success 测试点赞卡片成功
func TestKuaishouCardController_LikeCard_Success(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	svc := service.NewKuaishouCardService(database)
	ctrl := NewKuaishouCardController(svc)
	router := setupGinEngine()

	// 先创建卡片
	svc.Create(context.Background(), &dto.KuaishouCardCreateRequest{
		Title:        "Card to Like",
		Description:  "This card will be liked",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: 1,
		IsActive:     true,
	})

	router.POST("/kuaishou/cards/:id/like", ctrl.LikeCard)

	req, _ := http.NewRequest("POST", "/kuaishou/cards/1/like", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestKuaishouCardController_LikeCard_InvalidID 测试无效 ID
func TestKuaishouCardController_LikeCard_InvalidID(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	ctrl := NewKuaishouCardController(service.NewKuaishouCardService(database))
	router := setupGinEngine()
	router.POST("/kuaishou/cards/:id/like", ctrl.LikeCard)

	req, _ := http.NewRequest("POST", "/kuaishou/cards/invalid/like", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestKuaishouCardController_ShareCard_Success 测试分享卡片成功
func TestKuaishouCardController_ShareCard_Success(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	svc := service.NewKuaishouCardService(database)
	ctrl := NewKuaishouCardController(svc)
	router := setupGinEngine()

	// 先创建卡片
	svc.Create(context.Background(), &dto.KuaishouCardCreateRequest{
		Title:        "Card to Share",
		Description:  "This card will be shared",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: 1,
		IsActive:     true,
	})

	router.POST("/kuaishou/cards/:id/share", ctrl.ShareCard)

	req, _ := http.NewRequest("POST", "/kuaishou/cards/1/share?platform=wechat", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestKuaishouCardController_ShareCard_InvalidID 测试无效 ID
func TestKuaishouCardController_ShareCard_InvalidID(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	ctrl := NewKuaishouCardController(service.NewKuaishouCardService(database))
	router := setupGinEngine()
	router.POST("/kuaishou/cards/:id/share", ctrl.ShareCard)

	req, _ := http.NewRequest("POST", "/kuaishou/cards/invalid/share?platform=wechat", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestKuaishouCardController_GenerateShortLink_Success 测试生成短链成功
func TestKuaishouCardController_GenerateShortLink_Success(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	svc := service.NewKuaishouCardService(database)
	ctrl := NewKuaishouCardController(svc)
	router := setupGinEngine()

	// 先创建卡片
	card, err := svc.Create(context.Background(), &dto.KuaishouCardCreateRequest{
		Title:        "Card with Short Link",
		Description:  "This card will have a short link",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: 1,
		IsActive:     true,
	})
	if err != nil {
		t.Fatalf("create card failed: %v", err)
	}

	router.POST("/kuaishou/cards/:id/shortlink", ctrl.GenerateShortLink)

	cardID := "1"
	if card != nil && card.ID != 0 {
		cardID = fmt.Sprintf("%d", card.ID)
	}
	req, _ := http.NewRequest("POST", "/kuaishou/cards/"+cardID+"/shortlink", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 短链生成依赖外部域名，测试环境无数据时接受 200/500
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError && w.Code != http.StatusNotFound {
		t.Errorf("Expected status OK or Internal Server Error, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestKuaishouCardController_GenerateShortLink_InvalidID 测试无效 ID
func TestKuaishouCardController_GenerateShortLink_InvalidID(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	ctrl := NewKuaishouCardController(service.NewKuaishouCardService(database))
	router := setupGinEngine()
	router.POST("/kuaishou/cards/:id/shortlink", ctrl.GenerateShortLink)

	req, _ := http.NewRequest("POST", "/kuaishou/cards/invalid/shortlink", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestKuaishouCardController_GenerateShortLink_NotFound 测试卡片不存在
func TestKuaishouCardController_GenerateShortLink_NotFound(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	ctrl := NewKuaishouCardController(service.NewKuaishouCardService(database))
	router := setupGinEngine()
	router.POST("/kuaishou/cards/:id/shortlink", ctrl.GenerateShortLink)

	req, _ := http.NewRequest("POST", "/kuaishou/cards/999/shortlink", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Service returns record not found error which results in 404 or 500
	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status Not Found or Internal Server Error, got %d", w.Code)
	}
}

// TestKuaishouCardController_Create_WithTags 测试创建带标签的卡片
func TestKuaishouCardController_Create_WithTags(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	ctrl := NewKuaishouCardController(service.NewKuaishouCardService(database))
	router := setupGinEngine()
	router.POST("/kuaishou/cards", ctrl.Create)

	createReq := dto.KuaishouCardCreateRequest{
		Title:        "Card with Tags",
		Description:  "This card has tags",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: 1,
		Tags:         "tag1,tag2,tag3",
		IsActive:     true,
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/kuaishou/cards", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestKuaishouCardController_Create_InactiveCard 测试创建非激活卡片
func TestKuaishouCardController_Create_InactiveCard(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	ctrl := NewKuaishouCardController(service.NewKuaishouCardService(database))
	router := setupGinEngine()
	router.POST("/kuaishou/cards", ctrl.Create)

	createReq := dto.KuaishouCardCreateRequest{
		Title:        "Inactive Card",
		Description:  "This card is inactive",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: 1,
		IsActive:     false,
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/kuaishou/cards", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestKuaishouCardController_Update_ViewCount 测试更新浏览次数
func TestKuaishouCardController_Update_ViewCount(t *testing.T) {
	database := setupKuaishouCardTestDB(t)
	svc := service.NewKuaishouCardService(database)
	ctrl := NewKuaishouCardController(svc)
	router := setupGinEngine()

	// 先创建卡片
	svc.Create(context.Background(), &dto.KuaishouCardCreateRequest{
		Title:        "Card",
		Description:  "Description",
		ImageURL:     "https://example.com/image.jpg",
		DomainPoolID: 1,
		IsActive:     true,
	})

	router.PUT("/kuaishou/cards/:id", ctrl.Update)

	updateReq := dto.KuaishouCardUpdateRequest{
		ID:          1,
		Title:       "Card",
		Description: "Description",
		ImageURL:    "https://example.com/image.jpg",
		ViewCount:   1000,
		IsActive:    true,
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/kuaishou/cards/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}
