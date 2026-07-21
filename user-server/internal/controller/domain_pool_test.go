package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

func setupDomainPoolTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.DomainPool{},
	)
	db.SetTestDB(database)
	return database
}

func setupDomainPoolController(t *testing.T) (*DomainPoolController, *gin.Engine) {
	setupDomainPoolTestDB(t)
	db := db.GetDB()
	ctrl := NewDomainPoolController(service.NewDomainPoolService(db))
	router := gin.New()

	return ctrl, router
}

// TestDomainPoolController_Create_Success 测试创建域名池成功
func TestDomainPoolController_Create_Success(t *testing.T) {
	ctrl, router := setupDomainPoolController(t)
	router.POST("/domain-pool", ctrl.Create)

	createReq := dto.DomainPoolCreateRequest{
		Domain:  "test.example.com",
		Port:    8080,
		Purpose: "测试域名",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/domain-pool", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestDomainPoolController_Create_InvalidJSON 测试无效 JSON
func TestDomainPoolController_Create_InvalidJSON(t *testing.T) {
	ctrl, router := setupDomainPoolController(t)
	router.POST("/domain-pool", ctrl.Create)

	req, _ := http.NewRequest("POST", "/domain-pool", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestDomainPoolController_Create_MissingDomain 测试缺少必填字段
func TestDomainPoolController_Create_MissingDomain(t *testing.T) {
	ctrl, router := setupDomainPoolController(t)
	router.POST("/domain-pool", ctrl.Create)

	createReq := dto.DomainPoolCreateRequest{
		Port:    8080,
		Purpose: "测试域名",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/domain-pool", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestDomainPoolController_Update_Success 测试更新域名池成功
func TestDomainPoolController_Update_Success(t *testing.T) {
	database := setupDomainPoolTestDB(t)
	ctrl := NewDomainPoolController(service.NewDomainPoolService(database))
	router := gin.New()

	// 先创建一条记录
	domainPool := model.DomainPool{
		Domain:  "test.example.com",
		Port:    8080,
		Purpose: "测试域名",
	}
	database.Create(&domainPool)

	router.PUT("/domain-pool", ctrl.Update)

	updateReq := dto.DomainPoolUpdateRequest{
		ID:      domainPool.ID,
		Domain:  "updated.example.com",
		Port:    9090,
		Purpose: "更新后的用途",
		Status:  1,
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/domain-pool", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestDomainPoolController_Update_InvalidJSON 测试无效 JSON
func TestDomainPoolController_Update_InvalidJSON(t *testing.T) {
	ctrl, router := setupDomainPoolController(t)
	router.PUT("/domain-pool", ctrl.Update)

	req, _ := http.NewRequest("PUT", "/domain-pool", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestDomainPoolController_Delete_Success 测试删除域名池成功
func TestDomainPoolController_Delete_Success(t *testing.T) {
	database := setupDomainPoolTestDB(t)
	ctrl := NewDomainPoolController(service.NewDomainPoolService(database))
	router := gin.New()

	// 先创建一条记录
	domainPool := model.DomainPool{
		Domain:  "test.example.com",
		Port:    8080,
		Purpose: "测试域名",
	}
	database.Create(&domainPool)

	router.DELETE("/domain-pool/:id", ctrl.Delete)

	req, _ := http.NewRequest("DELETE", fmt.Sprintf("/domain-pool/%d", domainPool.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestDomainPoolController_Delete_InvalidID 测试无效 ID
func TestDomainPoolController_Delete_InvalidID(t *testing.T) {
	ctrl, router := setupDomainPoolController(t)
	router.DELETE("/domain-pool/:id", ctrl.Delete)

	req, _ := http.NewRequest("DELETE", "/domain-pool/invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestDomainPoolController_GetByID_Success 测试获取域名池详情成功
func TestDomainPoolController_GetByID_Success(t *testing.T) {
	database := setupDomainPoolTestDB(t)
	ctrl := NewDomainPoolController(service.NewDomainPoolService(database))
	router := gin.New()

	// 先创建一条记录
	domainPool := model.DomainPool{
		Domain:  "test.example.com",
		Port:    8080,
		Purpose: "测试域名",
	}
	database.Create(&domainPool)

	router.GET("/domain-pool/:id", ctrl.GetByID)

	req, _ := http.NewRequest("GET", fmt.Sprintf("/domain-pool/%d", domainPool.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestDomainPoolController_GetByID_InvalidID 测试无效 ID
func TestDomainPoolController_GetByID_InvalidID(t *testing.T) {
	ctrl, router := setupDomainPoolController(t)
	router.GET("/domain-pool/:id", ctrl.GetByID)

	req, _ := http.NewRequest("GET", "/domain-pool/invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestDomainPoolController_GetByID_NotFound 测试域名池不存在
func TestDomainPoolController_GetByID_NotFound(t *testing.T) {
	ctrl, router := setupDomainPoolController(t)
	router.GET("/domain-pool/:id", ctrl.GetByID)

	req, _ := http.NewRequest("GET", "/domain-pool/999999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 返回 500 是因为 GetByID 返回错误
	if w.Code != http.StatusInternalServerError && w.Code != http.StatusNotFound {
		t.Errorf("Expected status Not Found or Internal Server Error, got %d", w.Code)
	}
}

// TestDomainPoolController_List_Success 测试获取域名池列表成功
func TestDomainPoolController_List_Success(t *testing.T) {
	database := setupDomainPoolTestDB(t)
	ctrl := NewDomainPoolController(service.NewDomainPoolService(database))
	router := gin.New()

	// 创建测试数据
	domainPools := []model.DomainPool{
		{Domain: "test1.example.com", Port: 8080, Purpose: "测试域名 1"},
		{Domain: "test2.example.com", Port: 9090, Purpose: "测试域名 2"},
	}
	for _, dp := range domainPools {
		database.Create(&dp)
	}

	router.GET("/domain-pool", ctrl.List)

	req, _ := http.NewRequest("GET", "/domain-pool?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestDomainPoolController_List_EmptyList 测试空列表
func TestDomainPoolController_List_EmptyList(t *testing.T) {
	setupDomainPoolTestDB(t)
	ctrl, router := setupDomainPoolController(t)
	router.GET("/domain-pool", ctrl.List)

	req, _ := http.NewRequest("GET", "/domain-pool?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestDomainPoolController_List_WithFilters 测试带过滤条件的列表
func TestDomainPoolController_List_WithFilters(t *testing.T) {
	database := setupDomainPoolTestDB(t)
	ctrl := NewDomainPoolController(service.NewDomainPoolService(database))
	router := gin.New()

	// 创建测试数据
	domainPools := []model.DomainPool{
		{Domain: "test1.example.com", Port: 8080, Purpose: "测试域名 1", Status: 1},
		{Domain: "test2.example.com", Port: 9090, Purpose: "测试域名 2", Status: 2},
	}
	for _, dp := range domainPools {
		database.Create(&dp)
	}

	router.GET("/domain-pool", ctrl.List)

	req, _ := http.NewRequest("GET", "/domain-pool?page=1&page_size=10&domain=test1&status=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestDomainPoolController_CheckDomain_Success 测试检查域名成功
func TestDomainPoolController_CheckDomain_Success(t *testing.T) {
	database := setupDomainPoolTestDB(t)
	ctrl := NewDomainPoolController(service.NewDomainPoolService(database))
	router := gin.New()

	// 先创建一条记录
	domainPool := model.DomainPool{
		Domain:  "invalid.example.com",
		Port:    8080,
		Purpose: "测试域名",
	}
	database.Create(&domainPool)

	router.GET("/domain-pool/:id/check", ctrl.CheckDomain)

	req, _ := http.NewRequest("GET", fmt.Sprintf("/domain-pool/%d/check", domainPool.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 由于域名不可访问，应该返回 OK 但状态为 2
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status OK or Internal Server Error, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestDomainPoolController_CheckDomain_InvalidID 测试无效 ID
func TestDomainPoolController_CheckDomain_InvalidID(t *testing.T) {
	ctrl, router := setupDomainPoolController(t)
	router.GET("/domain-pool/:id/check", ctrl.CheckDomain)

	req, _ := http.NewRequest("GET", "/domain-pool/invalid/check", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestDomainPoolController_CheckAllDomains_Success 测试检查所有域名成功
func TestDomainPoolController_CheckAllDomains_Success(t *testing.T) {
	database := setupDomainPoolTestDB(t)
	ctrl := NewDomainPoolController(service.NewDomainPoolService(database))
	router := gin.New()

	// 先创建一条记录
	domainPool := model.DomainPool{
		Domain:  "invalid.example.com",
		Port:    8080,
		Purpose: "测试域名",
	}
	database.Create(&domainPool)

	router.GET("/domain-pool/check-all", ctrl.CheckAllDomains)

	req, _ := http.NewRequest("GET", "/domain-pool/check-all", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 应该返回 OK
	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestDomainPoolController_NewDomainPoolController 测试构造函数
func TestDomainPoolController_NewDomainPoolController(t *testing.T) {
	database := setupDomainPoolTestDB(t)
	ctrl := NewDomainPoolController(service.NewDomainPoolService(database))
	if ctrl == nil {
		t.Error("Expected controller instance, got nil")
	}
}
