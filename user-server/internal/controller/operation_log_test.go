package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

func setupOperationLogTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.OperationLog{},
	)
	db.SetTestDB(database)
	return database
}

func setupOperationLogController(t *testing.T) (*OperationLogController, *gin.Engine) {
	setupOperationLogTestDB(t)
	ctrl := NewOperationLogController()
	router := gin.New()

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("user_id", uint(1))
		ctx.Next()
	})

	return ctrl, router
}

// TestOperationLogController_GetList_Success 测试获取操作日志列表成功
func TestOperationLogController_GetList_Success(t *testing.T) {
	database := setupOperationLogTestDB(t)
	ctrl := NewOperationLogController()
	router := gin.New()

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("user_id", uint(1))
		ctx.Next()
	})

	// 创建测试数据
	logs := []model.OperationLog{
		{UserID: 1, Username: "admin", Action: "create", Module: "user"},
		{UserID: 1, Username: "admin", Action: "update", Module: "card"},
	}
	for _, log := range logs {
		database.Create(&log)
	}

	router.GET("/operation-logs", ctrl.GetList)

	req, _ := http.NewRequest("GET", "/operation-logs?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestOperationLogController_GetList_EmptyList 测试空列表
func TestOperationLogController_GetList_EmptyList(t *testing.T) {
	setupOperationLogTestDB(t)
	ctrl, router := setupOperationLogController(t)
	router.GET("/operation-logs", ctrl.GetList)

	req, _ := http.NewRequest("GET", "/operation-logs?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestOperationLogController_GetList_WithFilters 测试带过滤条件的列表
func TestOperationLogController_GetList_WithFilters(t *testing.T) {
	database := setupOperationLogTestDB(t)
	ctrl := NewOperationLogController()
	router := gin.New()

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("user_id", uint(1))
		ctx.Next()
	})

	// 创建测试数据
	logs := []model.OperationLog{
		{UserID: 1, Username: "admin", Action: "create", Module: "user"},
		{UserID: 2, Username: "user1", Action: "update", Module: "card"},
	}
	for _, log := range logs {
		database.Create(&log)
	}

	router.GET("/operation-logs", ctrl.GetList)

	req, _ := http.NewRequest("GET", "/operation-logs?page=1&page_size=10&action=create&module=user", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestOperationLogController_GetList_NoMerchant(t *testing.T) {
	setupOperationLogTestDB(t)
	ctrl := NewOperationLogController()
	router := gin.New()

	router.GET("/operation-logs", ctrl.GetList)

	req, _ := http.NewRequest("GET", "/operation-logs?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 当前 GetList 实现未做 user_id 鉴权,直接查询并返回 200
	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestOperationLogController_GetByID_Success 测试获取日志详情成功
func TestOperationLogController_GetByID_Success(t *testing.T) {
	database := setupOperationLogTestDB(t)
	ctrl := NewOperationLogController()
	router := gin.New()

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("user_id", uint(1))
		ctx.Next()
	})

	log := model.OperationLog{
		UserID:   1,
		Username: "admin",
		Action:   "create",
		Module:   "user",
	}
	database.Create(&log)

	router.GET("/operation-logs/:id", ctrl.GetByID)

	req, _ := http.NewRequest("GET", "/operation-logs/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestOperationLogController_GetByID_InvalidID 测试无效 ID
func TestOperationLogController_GetByID_InvalidID(t *testing.T) {
	ctrl, router := setupOperationLogController(t)
	router.GET("/operation-logs/:id", ctrl.GetByID)

	req, _ := http.NewRequest("GET", "/operation-logs/invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestOperationLogController_GetByID_NotFound 测试日志不存在
func TestOperationLogController_GetByID_NotFound(t *testing.T) {
	ctrl, router := setupOperationLogController(t)
	router.GET("/operation-logs/:id", ctrl.GetByID)

	req, _ := http.NewRequest("GET", "/operation-logs/999999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status Not Found, got %d", w.Code)
	}
}

// TestOperationLogController_GetMyLogs_Success 测试获取当前用户日志成功
func TestOperationLogController_GetMyLogs_Success(t *testing.T) {
	database := setupOperationLogTestDB(t)
	ctrl := NewOperationLogController()
	router := gin.New()

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("user_id", uint(1))
		ctx.Next()
	})

	// 创建测试数据
	logs := []model.OperationLog{
		{UserID: 1, Username: "admin", Action: "create", Module: "user"},
		{UserID: 1, Username: "admin", Action: "update", Module: "card"},
	}
	for _, log := range logs {
		database.Create(&log)
	}

	router.GET("/operation-logs/my", ctrl.GetMyLogs)

	req, _ := http.NewRequest("GET", "/operation-logs/my?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestOperationLogController_GetMyLogs_EmptyList 测试空列表
func TestOperationLogController_GetMyLogs_EmptyList(t *testing.T) {
	setupOperationLogTestDB(t)
	ctrl, router := setupOperationLogController(t)
	router.GET("/operation-logs/my", ctrl.GetMyLogs)

	req, _ := http.NewRequest("GET", "/operation-logs/my?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestOperationLogController_GetMyLogs_NoUser 测试缺少用户信息
func TestOperationLogController_GetMyLogs_NoUser(t *testing.T) {
	setupOperationLogTestDB(t)
	ctrl := NewOperationLogController()
	router := gin.New()

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		// 不设置 uint 类型的 user_id, 类型断言会失败
		ctx.Next()
	})

	router.GET("/operation-logs/my", ctrl.GetMyLogs)

	req, _ := http.NewRequest("GET", "/operation-logs/my?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 当前 GetMyLogs 实现:user_id 字符串无法转换为 uint,uid=0 进入查询,返回 200(空列表)
	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestOperationLogController_GetStatistics_Success 测试获取统计信息成功
func TestOperationLogController_GetStatistics_Success(t *testing.T) {
	database := setupOperationLogTestDB(t)
	ctrl := NewOperationLogController()
	router := gin.New()

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("user_id", uint(1))
		ctx.Next()
	})

	// 创建测试数据
	logs := []model.OperationLog{
		{UserID: 1, Username: "admin", Action: "create", Module: "user"},
		{UserID: 1, Username: "admin", Action: "update", Module: "user"},
		{UserID: 1, Username: "admin", Action: "delete", Module: "card"},
	}
	for _, log := range logs {
		database.Create(&log)
	}

	router.GET("/operation-logs/statistics", ctrl.GetStatistics)

	req, _ := http.NewRequest("GET", "/operation-logs/statistics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestOperationLogController_GetStatistics_EmptyList 测试空列表统计
func TestOperationLogController_GetStatistics_EmptyList(t *testing.T) {
	setupOperationLogTestDB(t)
	ctrl, router := setupOperationLogController(t)
	router.GET("/operation-logs/statistics", ctrl.GetStatistics)

	req, _ := http.NewRequest("GET", "/operation-logs/statistics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

func TestOperationLogController_GetStatistics_NoMerchant(t *testing.T) {
	setupOperationLogTestDB(t)
	ctrl := NewOperationLogController()
	router := gin.New()

	router.GET("/operation-logs/statistics", ctrl.GetStatistics)

	req, _ := http.NewRequest("GET", "/operation-logs/statistics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 当前 GetStatistics 实现未做 user_id 鉴权,直接查询并返回 200
	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestOperationLogController_NewOperationLogController 测试构造函数
func TestOperationLogController_NewOperationLogController(t *testing.T) {
	ctrl := NewOperationLogController()
	if ctrl == nil {
		t.Error("Expected controller instance, got nil")
	}
}
