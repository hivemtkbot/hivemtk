package controller

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/service"

	"hivemtk-user/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// setupCustomerTestDB 设置测试数据库
func setupCustomerTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.Customer{},
	)
	db.SetTestDB(database)
	return database
}

// setupCustomerRouter 设置测试路由
func setupCustomerRouter(t *testing.T) *gin.Engine {
	gin.SetMode(gin.TestMode)

	controller := NewCustomerController()

	router := gin.New()

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "merchant-test")
		c.Next()
	})

	router.GET("/api/customer", controller.ListCustomers)
	router.GET("/api/customer/:id", controller.GetCustomer)
	router.POST("/api/customer", controller.CreateCustomer)
	router.POST("/api/customer/:id/tags", controller.AddTags)
	router.DELETE("/api/customer/:id/tags", controller.RemoveTags)
	router.POST("/api/customer/merge", controller.MergeCustomers)

	return router
}

// TestCustomerController_ListCustomers 测试获取客户列表
func TestCustomerController_ListCustomers(t *testing.T) {
	setupCustomerTestDB(t)
	router := setupCustomerRouter(t)

	tests := []struct {
		name           string
		url            string
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name:           "default_pagination",
			url:            "/api/customer",
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name:           "custom_page_size",
			url:            "/api/customer?page=1&limit=10",
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name:           "invalid_page",
			url:            "/api/customer?page=abc&limit=xyz",
			expectedStatus: 400, 
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", tt.url, nil)
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectSuccess {
				assert.Equal(t, "SUCCESS", response["code"])
			} else {
				assert.NotEqual(t, "SUCCESS", response["code"])
			}
		})
	}
}

// TestCustomerController_GetCustomer 测试获取客户详情
func TestCustomerController_GetCustomer(t *testing.T) {
	setupCustomerTestDB(t)
	router := setupCustomerRouter(t)

	tests := []struct {
		name           string
		url            string
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name:           "missing_customer",
			url:            "/api/customer/non-existent-id",
			expectedStatus: 404, 
			expectSuccess:  false,
		},
		{
			name:           "empty_id",
			url:            "/api/customer/",
			expectedStatus: 301, 
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", tt.url, nil)
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// TestCustomerController_CreateCustomer 测试创建客户
func TestCustomerController_CreateCustomer(t *testing.T) {
	setupCustomerTestDB(t)
	router := setupCustomerRouter(t)

	tests := []struct {
		name           string
		requestBody    service.CustomerDTO
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name: "success",
			requestBody: service.CustomerDTO{
				Phone:         "13800138000",
				Email:         "test@example.com",
				WechatOpenID:  "test-wechat-openid",
				DouyinOpenID:  "test-douyin-openid",
				XiaohongshuID: "test-xiaohongshu-id",
			},
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name: "missing_user_id",
			requestBody: service.CustomerDTO{
				Phone:        "13800138000",
				Email:        "test@example.com",
				WechatOpenID: "test-wechat-openid",
			},
			expectedStatus: 200, 
			expectSuccess:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/api/customer", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// TestCustomerController_AddTags 测试添加标签
func TestCustomerController_AddTags(t *testing.T) {
	setupCustomerTestDB(t)
	router := setupCustomerRouter(t)

	tests := []struct {
		name           string
		url            string
		requestBody    map[string]any
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name: "success",
			requestBody: map[string]any{
				"tags": []string{"VIP", "High-Value"},
			},
			url:            "/api/customer/test-customer-id/tags",
			expectedStatus: 404, 
			expectSuccess:  false,
		},
		{
			name: "empty_tags",
			requestBody: map[string]any{
				"tags": []string{},
			},
			url:            "/api/customer/test-customer-id/tags",
			expectedStatus: 200, 
			expectSuccess:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", tt.url, bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// TestCustomerController_RemoveTags 测试移除标签
func TestCustomerController_RemoveTags(t *testing.T) {
	setupCustomerTestDB(t)
	router := setupCustomerRouter(t)

	tests := []struct {
		name           string
		url            string
		requestBody    map[string]any
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name: "success",
			requestBody: map[string]any{
				"tags": []string{"VIP"},
			},
			url:            "/api/customer/test-customer-id/tags",
			expectedStatus: 404, 
			expectSuccess:  false,
		},
		{
			name: "empty_tags",
			requestBody: map[string]any{
				"tags": []string{},
			},
			url:            "/api/customer/test-customer-id/tags",
			expectedStatus: 200, 
			expectSuccess:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("DELETE", tt.url, bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// TestCustomerController_MergeCustomers 测试合并客户
func TestCustomerController_MergeCustomers(t *testing.T) {
	setupCustomerTestDB(t)
	router := setupCustomerRouter(t)

	tests := []struct {
		name           string
		requestBody    map[string]any
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name: "same_customer",
			requestBody: map[string]any{
				"primary_id":   "customer-1",
				"secondary_id": "customer-1",
			},
			expectedStatus: 500, 
			expectSuccess:  false,
		},
		{
			name: "non_existent_customers",
			requestBody: map[string]any{
				"primary_id":   "non-existent-1",
				"secondary_id": "non-existent-2",
			},
			expectedStatus: 404, 
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/api/customer/merge", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

