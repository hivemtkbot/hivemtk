package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sysmodel "hivemtk-user/internal/model"
	"hivemtk-user/internal/ops/model"
	"hivemtk-user/internal/ops/service"
	"hivemtk-user/internal/pkg/db"

	"hivemtk-user/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupABExperimentTestDB 设置 A/B 实验测试数据库
func setupABExperimentTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&sysmodel.SystemUser{},
		&sysmodel.User{},
		&sysmodel.Account{},
		&model.ABExperiment{},
		&model.ABVariant{},
		&model.ABConversionEvent{},
		&model.ABExperimentResult{},
	)
	db.SetTestDB(database)
	return database
}

// setupABExperimentController 设置 A/B 实验控制器测试环境
func setupABExperimentController(t *testing.T) (*ABExperimentController, *gin.Engine) {
	setupABExperimentTestDB(t)
	ctrl := NewABExperimentController()
	router := gin.New()

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Next()
	})

	return ctrl, router
}

// TestABExperimentController_CreateExperiment_Success 测试创建实验成功
func TestABExperimentController_CreateExperiment_Success(t *testing.T) {
	ctrl, router := setupABExperimentController(t)
	router.POST("/experiments", ctrl.CreateExperiment)

	createReq := service.CreateExperimentRequest{
		Name:         "Test Experiment",
		Description:  "Test Description",
		SourceType:   "page",
		SourceID:     "page_123",
		TrafficSplit: 50,
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/experiments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestABExperimentController_CreateExperiment_InvalidJSON 测试无效 JSON
func TestABExperimentController_CreateExperiment_InvalidJSON(t *testing.T) {
	ctrl, router := setupABExperimentController(t)
	router.POST("/experiments", ctrl.CreateExperiment)

	req, _ := http.NewRequest("POST", "/experiments", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestABExperimentController_CreateExperiment_MissingRequiredFields 测试缺少必填字段
func TestABExperimentController_CreateExperiment_MissingRequiredFields(t *testing.T) {
	ctrl, router := setupABExperimentController(t)
	router.POST("/experiments", ctrl.CreateExperiment)

	createReq := service.CreateExperimentRequest{
		Name: "", 
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/experiments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestABExperimentController_GetExperiment_Success 测试获取实验详情成功
func TestABExperimentController_GetExperiment_Success(t *testing.T) {
	database := setupABExperimentTestDB(t)
	ctrl := NewABExperimentController()
	router := gin.New()

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Next()
	})

	experiment := model.ABExperiment{
		Name:        "Test Experiment",
		Description: "Test Description",
		Status:      "draft",
		SourceType:  "page",
	}
	database.Create(&experiment)

	router.POST("/experiments", ctrl.CreateExperiment)
	router.GET("/experiments/:id", ctrl.GetExperiment)

	req, _ := http.NewRequest("GET", "/experiments/"+fmt.Sprintf("%d", experiment.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestABExperimentController_GetExperiment_InvalidID 测试无效实验 ID
func TestABExperimentController_GetExperiment_InvalidID(t *testing.T) {
	ctrl, router := setupABExperimentController(t)
	router.GET("/experiments/:id", ctrl.GetExperiment)

	req, _ := http.NewRequest("GET", "/experiments/invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestABExperimentController_GetExperiment_NotFound 测试实验不存在
func TestABExperimentController_GetExperiment_NotFound(t *testing.T) {
	ctrl, router := setupABExperimentController(t)
	router.GET("/experiments/:id", ctrl.GetExperiment)

	req, _ := http.NewRequest("GET", "/experiments/999999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status Not Found, got %d", w.Code)
	}
}

// TestABExperimentController_GetExperimentList_Success 测试获取实验列表成功
func TestABExperimentController_GetExperimentList_Success(t *testing.T) {
	database := setupABExperimentTestDB(t)
	ctrl, router := setupABExperimentController(t)

	experiments := []model.ABExperiment{
		{Name: "Experiment 1", Status: "running"},
		{Name: "Experiment 2", Status: "draft"},
		{Name: "Experiment 3", Status: "completed"},
	}
	for _, exp := range experiments {
		database.Create(&exp)
	}

	router.GET("/experiments", ctrl.GetExperimentList)

	req, _ := http.NewRequest("GET", "/experiments?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestABExperimentController_GetExperimentList_EmptyList 测试空实验列表
func TestABExperimentController_GetExperimentList_EmptyList(t *testing.T) {
	ctrl, router := setupABExperimentController(t)
	router.GET("/experiments", ctrl.GetExperimentList)

	req, _ := http.NewRequest("GET", "/experiments?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestABExperimentController_UpdateExperiment_Success 测试更新实验成功
func TestABExperimentController_UpdateExperiment_Success(t *testing.T) {
	database := setupABExperimentTestDB(t)
	ctrl := NewABExperimentController()
	router := gin.New()

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Next()
	})

	experiment := model.ABExperiment{
		Name:        "Test Experiment",
		Description: "Test Description",
		Status:      "draft",
		SourceType:  "page",
		SourceID:    "page_123",
	}
	database.Create(&experiment)

	router.PUT("/experiments/:id", ctrl.UpdateExperiment)

	updateReq := service.CreateExperimentRequest{
		Name:         "Updated Experiment",
		Description:  "Updated Description",
		SourceType:   "component",
		SourceID:     "component_456",
		TrafficSplit: 60,
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/experiments/"+fmt.Sprintf("%d", experiment.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestABExperimentController_UpdateExperiment_InvalidID 测试无效实验 ID
func TestABExperimentController_UpdateExperiment_InvalidID(t *testing.T) {
	ctrl, router := setupABExperimentController(t)
	router.PUT("/experiments/:id", ctrl.UpdateExperiment)

	updateReq := service.CreateExperimentRequest{
		Name: "Updated Experiment",
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/experiments/invalid", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestABExperimentController_UpdateExperiment_InvalidJSON 测试无效 JSON
func TestABExperimentController_UpdateExperiment_InvalidJSON(t *testing.T) {
	database := setupABExperimentTestDB(t)
	ctrl, router := setupABExperimentController(t)

	experiment := model.ABExperiment{
		Name: "Test Experiment",
	}
	database.Create(&experiment)

	router.PUT("/experiments/:id", ctrl.UpdateExperiment)

	req, _ := http.NewRequest("PUT", "/experiments/1", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestABExperimentController_DeleteExperiment_Success 测试删除实验成功
func TestABExperimentController_DeleteExperiment_Success(t *testing.T) {
	database := setupABExperimentTestDB(t)
	ctrl, router := setupABExperimentController(t)

	experiment := model.ABExperiment{
		Name:   "Test Experiment",
		Status: "draft",
	}
	database.Create(&experiment)

	router.DELETE("/experiments/:id", ctrl.DeleteExperiment)

	req, _ := http.NewRequest("DELETE", "/experiments/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestABExperimentController_DeleteExperiment_InvalidID 测试无效实验 ID
func TestABExperimentController_DeleteExperiment_InvalidID(t *testing.T) {
	ctrl, router := setupABExperimentController(t)
	router.DELETE("/experiments/:id", ctrl.DeleteExperiment)

	req, _ := http.NewRequest("DELETE", "/experiments/invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestABExperimentController_StartExperiment_Success 测试启动实验成功
func TestABExperimentController_StartExperiment_Success(t *testing.T) {
	database := setupABExperimentTestDB(t)
	ctrl, router := setupABExperimentController(t)

	experiment := model.ABExperiment{
		Name:   "Test Experiment",
		Status: "draft",
	}
	database.Create(&experiment)

	router.POST("/experiments/:id/start", ctrl.StartExperiment)

	req, _ := http.NewRequest("POST", "/experiments/1/start", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestABExperimentController_StartExperiment_InvalidID 测试无效实验 ID
func TestABExperimentController_StartExperiment_InvalidID(t *testing.T) {
	ctrl, router := setupABExperimentController(t)
	router.POST("/experiments/:id/start", ctrl.StartExperiment)

	req, _ := http.NewRequest("POST", "/experiments/invalid/start", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestABExperimentController_PauseExperiment_Success 测试暂停实验成功
func TestABExperimentController_PauseExperiment_Success(t *testing.T) {
	database := setupABExperimentTestDB(t)
	ctrl, router := setupABExperimentController(t)

	experiment := model.ABExperiment{
		Name:   "Test Experiment",
		Status: "running",
	}
	database.Create(&experiment)

	router.POST("/experiments/:id/pause", ctrl.PauseExperiment)

	req, _ := http.NewRequest("POST", "/experiments/1/pause", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestABExperimentController_PauseExperiment_InvalidID 测试无效实验 ID
func TestABExperimentController_PauseExperiment_InvalidID(t *testing.T) {
	ctrl, router := setupABExperimentController(t)
	router.POST("/experiments/:id/pause", ctrl.PauseExperiment)

	req, _ := http.NewRequest("POST", "/experiments/invalid/pause", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestABExperimentController_StopExperiment_Success 测试停止实验成功
func TestABExperimentController_StopExperiment_Success(t *testing.T) {
	database := setupABExperimentTestDB(t)
	ctrl, router := setupABExperimentController(t)

	experiment := model.ABExperiment{
		Name:   "Test Experiment",
		Status: "running",
	}
	database.Create(&experiment)

	router.POST("/experiments/:id/stop", ctrl.StopExperiment)

	req, _ := http.NewRequest("POST", "/experiments/1/stop", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestABExperimentController_StopExperiment_InvalidID 测试无效实验 ID
func TestABExperimentController_StopExperiment_InvalidID(t *testing.T) {
	ctrl, router := setupABExperimentController(t)
	router.POST("/experiments/:id/stop", ctrl.StopExperiment)

	req, _ := http.NewRequest("POST", "/experiments/invalid/stop", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestABExperimentController_GetExperimentResults_Success 测试获取实验结果成功
func TestABExperimentController_GetExperimentResults_Success(t *testing.T) {
	database := setupABExperimentTestDB(t)
	ctrl, router := setupABExperimentController(t)

	experiment := model.ABExperiment{
		Name:   "Test Experiment",
		Status: "completed",
	}
	database.Create(&experiment)

	router.GET("/experiments/:id/results", ctrl.GetExperimentResults)

	req, _ := http.NewRequest("GET", "/experiments/1/results", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestABExperimentController_GetExperimentResults_InvalidID 测试无效实验 ID
func TestABExperimentController_GetExperimentResults_InvalidID(t *testing.T) {
	ctrl, router := setupABExperimentController(t)
	router.GET("/experiments/:id/results", ctrl.GetExperimentResults)

	req, _ := http.NewRequest("GET", "/experiments/invalid/results", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestABExperimentController_GetConversionEvents_Success 测试获取转化事件成功
func TestABExperimentController_GetConversionEvents_Success(t *testing.T) {
	database := setupABExperimentTestDB(t)
	ctrl, router := setupABExperimentController(t)

	experiment := model.ABExperiment{
		Name:   "Test Experiment",
		Status: "completed",
	}
	database.Create(&experiment)

	router.GET("/experiments/:id/conversions", ctrl.GetConversionEvents)

	req, _ := http.NewRequest("GET", "/experiments/1/conversions?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestABExperimentController_GetConversionEvents_InvalidID 测试无效实验 ID
func TestABExperimentController_GetConversionEvents_InvalidID(t *testing.T) {
	ctrl, router := setupABExperimentController(t)
	router.GET("/experiments/:id/conversions", ctrl.GetConversionEvents)

	req, _ := http.NewRequest("GET", "/experiments/invalid/conversions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestABExperimentController_CreateExperiment_WithAllFields 测试创建带所有字段的实验
func TestABExperimentController_CreateExperiment_WithAllFields(t *testing.T) {
	ctrl, router := setupABExperimentController(t)
	router.POST("/experiments", ctrl.CreateExperiment)

	now := time.Now().Format("2006-01-02")
	endDate := time.Now().AddDate(0, 0, 30).Format("2006-01-02")

	createReq := service.CreateExperimentRequest{
		Name:         "Complete Experiment",
		Description:  "Complete Description",
		SourceType:   "message",
		SourceID:     "message_456",
		TrafficSplit: 50,
		StartDate:    now,
		EndDate:      endDate,
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/experiments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestABExperimentController_CreateExperiment_WithDates 测试创建带日期设置的实验
func TestABExperimentController_CreateExperiment_WithDates(t *testing.T) {
	ctrl, router := setupABExperimentController(t)
	router.POST("/experiments", ctrl.CreateExperiment)

	now := time.Now().Format("2006-01-02")
	endDate := time.Now().AddDate(0, 0, 14).Format("2006-01-02")

	createReq := service.CreateExperimentRequest{
		Name:       "Dated Experiment",
		SourceType: "page",
		SourceID:   "page_789",
		StartDate:  now,
		EndDate:    endDate,
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/experiments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestABExperimentController_GetExperimentList_WithPagination 测试分页获取实验列表
func TestABExperimentController_GetExperimentList_WithPagination(t *testing.T) {
	database := setupABExperimentTestDB(t)
	ctrl, router := setupABExperimentController(t)

	for i := 0; i < 25; i++ {
		experiment := model.ABExperiment{
			Name:   "Experiment " + string(rune(i)),
			Status: "running",
		}
		database.Create(&experiment)
	}

	router.GET("/experiments", ctrl.GetExperimentList)

	req, _ := http.NewRequest("GET", "/experiments?page=2&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

