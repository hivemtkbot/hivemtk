package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/ops/model"
	"hivemtk-user/internal/pkg/db"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"hivemtk-user/internal/pkg/testutil"
)

func setupDashboardScreenTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.DashboardScreen{},
		&model.DashboardWidget{},
	)
	db.SetTestDB(database)
	return database
}

func setupDashboardScreenController(t *testing.T) (*DashboardScreenController, *gin.Engine) {
	setupDashboardScreenTestDB(t)
	ctrl := NewDashboardScreenController()
	router := gin.New()

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("user_id", uint(1))
		ctx.Next()
	})

	return ctrl, router
}

// TestDashboardScreenController_CreateScreen_Success 测试创建大屏成功
func TestDashboardScreenController_CreateScreen_Success(t *testing.T) {
	ctrl, router := setupDashboardScreenController(t)
	router.POST("/screens", ctrl.CreateScreen)

	createReq := map[string]any{
		"name":      "Test Screen",
		"theme":     "dark",
		"is_public": false,
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/screens", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestDashboardScreenController_CreateScreen_InvalidJSON 测试无效 JSON
func TestDashboardScreenController_CreateScreen_InvalidJSON(t *testing.T) {
	ctrl, router := setupDashboardScreenController(t)
	router.POST("/screens", ctrl.CreateScreen)

	req, _ := http.NewRequest("POST", "/screens", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestDashboardScreenController_CreateScreen_MissingName 测试缺少必填字段
func TestDashboardScreenController_CreateScreen_MissingName(t *testing.T) {
	ctrl, router := setupDashboardScreenController(t)
	router.POST("/screens", ctrl.CreateScreen)

	createReq := map[string]any{
		"theme":     "dark",
		"is_public": false,
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/screens", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestDashboardScreenController_GetScreenList_Success 测试获取大屏列表成功
func TestDashboardScreenController_GetScreenList_Success(t *testing.T) {
	database := setupDashboardScreenTestDB(t)
	ctrl := NewDashboardScreenController()
	router := gin.New()

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("user_id", uint(1))
		ctx.Next()
	})

	screens := []model.DashboardScreen{
		{Name: "Screen 1", Code: "code_1"},
		{Name: "Screen 2", Code: "code_2"},
	}
	for _, screen := range screens {
		database.Create(&screen)
	}

	router.GET("/screens", ctrl.GetScreenList)

	req, _ := http.NewRequest("GET", "/screens?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestDashboardScreenController_GetScreenList_EmptyList 测试空列表
func TestDashboardScreenController_GetScreenList_EmptyList(t *testing.T) {
	setupDashboardScreenTestDB(t)
	ctrl, router := setupDashboardScreenController(t)
	router.GET("/screens", ctrl.GetScreenList)

	req, _ := http.NewRequest("GET", "/screens?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestDashboardScreenController_GetScreenByID_Success 测试获取大屏详情成功
func TestDashboardScreenController_GetScreenByID_Success(t *testing.T) {
	database := setupDashboardScreenTestDB(t)
	ctrl := NewDashboardScreenController()
	router := gin.New()

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("user_id", uint(1))
		ctx.Next()
	})

	screen := model.DashboardScreen{
		Name:  "Test Screen",
		Code:  "test_code",
		Theme: "dark",
	}
	database.Create(&screen)

	router.GET("/screens/:id", ctrl.GetScreenByID)

	req, _ := http.NewRequest("GET", fmt.Sprintf("/screens/%d", screen.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestDashboardScreenController_GetScreenByID_InvalidID 测试无效 ID
func TestDashboardScreenController_GetScreenByID_InvalidID(t *testing.T) {
	ctrl, router := setupDashboardScreenController(t)
	router.GET("/screens/:id", ctrl.GetScreenByID)

	req, _ := http.NewRequest("GET", "/screens/invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestDashboardScreenController_GetScreenByID_NotFound 测试大屏不存在
func TestDashboardScreenController_GetScreenByID_NotFound(t *testing.T) {
	ctrl, router := setupDashboardScreenController(t)
	router.GET("/screens/:id", ctrl.GetScreenByID)

	req, _ := http.NewRequest("GET", "/screens/999999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status Not Found, got %d", w.Code)
	}
}

// TestDashboardScreenController_UpdateScreen_Success 测试更新大屏成功
func TestDashboardScreenController_UpdateScreen_Success(t *testing.T) {
	database := setupDashboardScreenTestDB(t)
	ctrl := NewDashboardScreenController()
	router := gin.New()

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("user_id", uint(1))
		ctx.Next()
	})

	screen := model.DashboardScreen{
		Name:  "Test Screen",
		Code:  "test_code",
		Theme: "dark",
	}
	database.Create(&screen)

	router.PUT("/screens/:id", ctrl.UpdateScreen)

	updateReq := map[string]any{
		"name":  "Updated Screen",
		"theme": "light",
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", fmt.Sprintf("/screens/%d", screen.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestDashboardScreenController_UpdateScreen_InvalidID 测试无效 ID
func TestDashboardScreenController_UpdateScreen_InvalidID(t *testing.T) {
	ctrl, router := setupDashboardScreenController(t)
	router.PUT("/screens/:id", ctrl.UpdateScreen)

	updateReq := map[string]any{
		"name": "Updated Screen",
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/screens/invalid", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestDashboardScreenController_UpdateScreen_InvalidJSON 测试无效 JSON
func TestDashboardScreenController_UpdateScreen_InvalidJSON(t *testing.T) {
	database := setupDashboardScreenTestDB(t)
	ctrl := NewDashboardScreenController()
	router := gin.New()

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("user_id", uint(1))
		ctx.Next()
	})

	screen := model.DashboardScreen{
		Name: "Test Screen",
	}
	database.Create(&screen)

	router.PUT("/screens/:id", ctrl.UpdateScreen)

	req, _ := http.NewRequest("PUT", fmt.Sprintf("/screens/%d", screen.ID), bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestDashboardScreenController_DeleteScreen_Success 测试删除大屏成功
func TestDashboardScreenController_DeleteScreen_Success(t *testing.T) {
	database := setupDashboardScreenTestDB(t)
	ctrl := NewDashboardScreenController()
	router := gin.New()

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("user_id", uint(1))
		ctx.Next()
	})

	screen := model.DashboardScreen{
		Name: "Test Screen",
		Code: "test_code",
	}
	database.Create(&screen)

	router.DELETE("/screens/:id", ctrl.DeleteScreen)

	req, _ := http.NewRequest("DELETE", fmt.Sprintf("/screens/%d", screen.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestDashboardScreenController_DeleteScreen_InvalidID 测试无效 ID
func TestDashboardScreenController_DeleteScreen_InvalidID(t *testing.T) {
	ctrl, router := setupDashboardScreenController(t)
	router.DELETE("/screens/:id", ctrl.DeleteScreen)

	req, _ := http.NewRequest("DELETE", "/screens/invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestDashboardScreenController_PublicViewScreen_Success 测试公开访问大屏成功
func TestDashboardScreenController_PublicViewScreen_Success(t *testing.T) {
	database := setupDashboardScreenTestDB(t)
	ctrl := NewDashboardScreenController()
	router := gin.New()

	screen := model.DashboardScreen{
		Name:     "Public Screen",
		Code:     "public_code",
		IsPublic: true,
	}
	database.Create(&screen)

	router.GET("/screens/public/:code", ctrl.PublicViewScreen)

	req, _ := http.NewRequest("GET", "/screens/public/public_code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestDashboardScreenController_PublicViewScreen_NotFound 测试大屏不存在
func TestDashboardScreenController_PublicViewScreen_NotFound(t *testing.T) {
	setupDashboardScreenTestDB(t)
	ctrl, router := setupDashboardScreenController(t)
	router.GET("/screens/public/:code", ctrl.PublicViewScreen)

	req, _ := http.NewRequest("GET", "/screens/public/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status Not Found, got %d", w.Code)
	}
}

// TestDashboardScreenController_NewDashboardScreenController 测试构造函数
func TestDashboardScreenController_NewDashboardScreenController(t *testing.T) {
	ctrl := NewDashboardScreenController()
	if ctrl == nil {
		t.Error("Expected controller instance, got nil")
	}
}
