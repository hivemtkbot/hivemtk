package controller

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"

	"hivemtk-user/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func setupCustomerEventTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.Customer{},
		&model.CustomerEvent{},
	)
	db.SetTestDB(database)
	return database
}

func setupCustomerEventRouter(t *testing.T) *gin.Engine {
	gin.SetMode(gin.TestMode)

	controller := NewCustomerEventController()

	router := gin.New()

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "merchant-test")
		c.Next()
	})

	router.POST("/api/events/track", controller.TrackEvent)
	router.GET("/api/events/customer/:id", controller.GetEventHistory)
	router.GET("/api/events/stats", controller.GetEventStats)
	router.POST("/api/events/pageview", controller.TrackPageView)
	router.POST("/api/events/click", controller.TrackClick)
	router.POST("/api/events/purchase", controller.TrackPurchase)
	router.POST("/api/events/signup", controller.TrackSignup)
	router.POST("/api/events/login", controller.TrackLogin)
	router.POST("/api/events/add-to-cart", controller.TrackAddToCart)

	return router
}

// TestCustomerEventController_TrackEvent 测试追踪事件
func TestCustomerEventController_TrackEvent(t *testing.T) {
	setupCustomerEventTestDB(t)
	router := setupCustomerEventRouter(t)

	tests := []struct {
		name           string
		requestBody    map[string]any
		expectedStatus int
		expectSuccess  bool
		allowSuccess   bool
	}{
		{
			name: "success",
			requestBody: map[string]any{
				"customer_id":  "customer-test",
				"user_id":      "merchant-test",
				"event_type":   "page_view",
				"event_source": "website",
				"event_data": map[string]any{
					"url":   "https://example.com",
					"title": "Home Page",
				},
			},
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name: "missing_customer_id",
			requestBody: map[string]any{
				"user_id":      "merchant-test",
				"event_type":   "page_view",
				"event_source": "website",
			},
			expectedStatus: 400,
			expectSuccess:  false,
		},
		{
			name: "missing_user_id",
			requestBody: map[string]any{
				"customer_id":  "customer-test-missing-user",
				"event_type":   "page_view",
				"event_source": "website",
			},
			expectedStatus: 0,
			expectSuccess:  false,
			allowSuccess:   true,
		},
		{
			name: "missing_event_type",
			requestBody: map[string]any{
				"customer_id":  "customer-test",
				"user_id":      "merchant-test",
				"event_source": "website",
			},
			expectedStatus: 400,
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/api/events/track", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(w, req)

			if tt.expectedStatus != 0 {
				assert.Equal(t, tt.expectedStatus, w.Code)
			} else {
				assert.True(t, w.Code == 200 || w.Code == 500,
					"expected 200 or 500, got %d", w.Code)
			}

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectSuccess {
				assert.Equal(t, float64(0), response["code"])
			} else if !tt.allowSuccess {
				assert.NotEqual(t, float64(0), response["code"])
			}
		})
	}
}

// TestCustomerEventController_GetEventHistory 测试获取事件历史
func TestCustomerEventController_GetEventHistory(t *testing.T) {
	setupCustomerEventTestDB(t)
	router := setupCustomerEventRouter(t)

	tests := []struct {
		name           string
		url            string
		expectedStatus int
		expectSuccess  bool
		allowSuccess   bool
	}{
		{
			name:           "success",
			url:            "/api/events/customer/customer-test",
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name:           "with_limit",
			url:            "/api/events/customer/customer-test?limit=10",
			expectedStatus: 200,
			expectSuccess:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", tt.url, nil)
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectSuccess {
				var response map[string]any
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, float64(0), response["code"])
			}
		})
	}
}

// TestCustomerEventController_GetEventStats 测试获取事件统计
func TestCustomerEventController_GetEventStats(t *testing.T) {
	setupCustomerEventTestDB(t)
	router := setupCustomerEventRouter(t)

	tests := []struct {
		name           string
		url            string
		expectedStatus int
		expectSuccess  bool
		allowSuccess   bool
	}{
		{
			name:           "success",
			url:            "/api/events/stats?start_date=2024-01-01",
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name:           "with_date_range",
			url:            "/api/events/stats?start_date=2024-01-01&start=2024-01-01&end=2024-12-31",
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name:           "missing_user_id",
			url:            "/api/events/stats",
			expectedStatus: 200,
			expectSuccess:  true,
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
				assert.Equal(t, float64(0), response["code"])
			} else if !tt.allowSuccess {
				assert.NotEqual(t, float64(0), response["code"])
			}
		})
	}
}

// TestCustomerEventController_TrackPageView 测试追踪页面浏览事件
func TestCustomerEventController_TrackPageView(t *testing.T) {
	setupCustomerEventTestDB(t)
	router := setupCustomerEventRouter(t)

	tests := []struct {
		name           string
		requestBody    map[string]any
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name: "success",
			requestBody: map[string]any{
				"customer_id": "customer-test",
				"user_id":     "merchant-test",
				"url":         "https://example.com",
				"title":       "Home Page",
			},
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name: "missing_url",
			requestBody: map[string]any{
				"customer_id": "customer-test",
				"user_id":     "merchant-test",
				"title":       "Home Page",
			},
			expectedStatus: 400,
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/api/events/pageview", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// TestCustomerEventController_TrackClick 测试追踪点击事件
func TestCustomerEventController_TrackClick(t *testing.T) {
	setupCustomerEventTestDB(t)
	router := setupCustomerEventRouter(t)

	tests := []struct {
		name           string
		requestBody    map[string]any
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name: "success",
			requestBody: map[string]any{
				"customer_id": "customer-test",
				"user_id":     "merchant-test",
				"element":     "button",
				"target":      "signup-btn",
			},
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name: "missing_element",
			requestBody: map[string]any{
				"customer_id": "customer-test",
				"user_id":     "merchant-test",
				"target":      "signup-btn",
			},
			expectedStatus: 400,
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/api/events/click", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// TestCustomerEventController_TrackPurchase 测试追踪购买事件
func TestCustomerEventController_TrackPurchase(t *testing.T) {
	setupCustomerEventTestDB(t)
	router := setupCustomerEventRouter(t)

	tests := []struct {
		name           string
		requestBody    map[string]any
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name: "success",
			requestBody: map[string]any{
				"customer_id": "customer-test",
				"user_id":     "merchant-test",
				"amount":      99.99,
				"items":       []any{"item1", "item2"},
			},
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name: "missing_amount",
			requestBody: map[string]any{
				"customer_id": "customer-test",
				"user_id":     "merchant-test",
				"items":       []any{"item1", "item2"},
			},
			expectedStatus: 400,
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/api/events/purchase", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// TestCustomerEventController_TrackSignup 测试追踪注册事件
func TestCustomerEventController_TrackSignup(t *testing.T) {
	setupCustomerEventTestDB(t)
	router := setupCustomerEventRouter(t)

	tests := []struct {
		name           string
		requestBody    map[string]any
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name: "success",
			requestBody: map[string]any{
				"customer_id":   "customer-test",
				"user_id":       "merchant-test",
				"signup_method": "email",
			},
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name: "missing_customer_id",
			requestBody: map[string]any{
				"user_id":       "merchant-test",
				"signup_method": "email",
			},
			expectedStatus: 400,
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/api/events/signup", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// TestCustomerEventController_TrackLogin 测试追踪登录事件
func TestCustomerEventController_TrackLogin(t *testing.T) {
	setupCustomerEventTestDB(t)
	router := setupCustomerEventRouter(t)

	tests := []struct {
		name           string
		requestBody    map[string]any
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name: "success",
			requestBody: map[string]any{
				"customer_id":  "customer-test",
				"user_id":      "merchant-test",
				"login_method": "password",
			},
			expectedStatus: 200,
			expectSuccess:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/api/events/login", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// TestCustomerEventController_TrackAddToCart 测试追踪加购事件
func TestCustomerEventController_TrackAddToCart(t *testing.T) {
	setupCustomerEventTestDB(t)
	router := setupCustomerEventRouter(t)

	tests := []struct {
		name           string
		requestBody    map[string]any
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name: "success",
			requestBody: map[string]any{
				"customer_id":  "customer-test",
				"user_id":      "merchant-test",
				"product_id":   "product-123",
				"product_name": "Test Product",
				"price":        29.99,
				"quantity":     2,
			},
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name: "missing_product_id",
			requestBody: map[string]any{
				"customer_id":  "customer-test",
				"user_id":      "merchant-test",
				"product_name": "Test Product",
				"price":        29.99,
				"quantity":     2,
			},
			expectedStatus: 400,
			expectSuccess:  false,
		},
		{
			name: "missing_price",
			requestBody: map[string]any{
				"customer_id":  "customer-test",
				"user_id":      "merchant-test",
				"product_id":   "product-123",
				"product_name": "Test Product",
				"quantity":     2,
			},
			expectedStatus: 400,
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/api/events/add-to-cart", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}
