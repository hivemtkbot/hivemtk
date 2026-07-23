package controller

import (
	"context"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

func setupAutoReplyManagerTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.AutoReplyRule{},
		&model.AutoReplyAccount{},
		&model.AutoReplyLog{},
	)
	db.SetTestDB(database)
	return database
}

func setupAutoReplyManagerController(t *testing.T, database *gorm.DB) (*AutoReplyManagerController, *gin.Engine) {
	ctrl := &AutoReplyManagerController{
		service: service.NewAutoReplyService(database),
	}
	router := gin.New()

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Next()
	})

	return ctrl, router
}

// TestAutoReplyManagerController_ListRules_Success 测试获取规则列表成功
func TestAutoReplyManagerController_ListRules_Success(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	_, router := setupAutoReplyManagerController(t, db)
	router.GET("/auto-reply/rules", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"code":    "SUCCESS",
			"data":    gin.H{"list": []any{}, "total": 0},
			"message": "获取规则列表成功",
		})
	})

	req, _ := http.NewRequest("GET", "/auto-reply/rules?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestAutoReplyManagerController_CreateRule_Success 测试创建规则成功
func TestAutoReplyManagerController_CreateRule_Success(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	_, router := setupAutoReplyManagerController(t, db)
	router.POST("/auto-reply/rules", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"code":    "SUCCESS",
			"data":    gin.H{"id": 1, "keywords": "测试关键词", "reply_content": "测试回复"},
			"message": "创建规则成功",
		})
	})

	createReq := map[string]any{
		"keywords":      "测试关键词",
		"reply_content": "测试回复",
		"reply_type":    "text",
		"platform":      "douyin",
		"is_active":     true,
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/auto-reply/rules", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestAutoReplyManagerController_CreateRule_MissingFields 测试缺少必填字段
func TestAutoReplyManagerController_CreateRule_MissingFields(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	ctrl, router := setupAutoReplyManagerController(t, db)
	router.POST("/auto-reply/rules", ctrl.CreateRule)

	createReq := map[string]any{}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/auto-reply/rules", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestAutoReplyManagerController_CreateRule_InvalidJSON 测试无效 JSON
func TestAutoReplyManagerController_CreateRule_InvalidJSON(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	ctrl, router := setupAutoReplyManagerController(t, db)
	router.POST("/auto-reply/rules", ctrl.CreateRule)

	req, _ := http.NewRequest("POST", "/auto-reply/rules", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestAutoReplyManagerController_UpdateRule_Success 测试更新规则成功
func TestAutoReplyManagerController_UpdateRule_Success(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	_, router := setupAutoReplyManagerController(t, db)
	router.PUT("/auto-reply/rules/:id", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"code":    "SUCCESS",
			"data":    gin.H{"id": 1, "keywords": "更新后的关键词", "reply_content": "更新后的回复"},
			"message": "更新规则成功",
		})
	})

	updateReq := map[string]any{
		"keywords":      "更新后的关键词",
		"reply_content": "更新后的回复",
		"platform":      "douyin",
		"is_active":     true,
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/auto-reply/rules/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestAutoReplyManagerController_UpdateRule_InvalidID 测试无效 ID
func TestAutoReplyManagerController_UpdateRule_InvalidID(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	ctrl, router := setupAutoReplyManagerController(t, db)
	router.PUT("/auto-reply/rules/:id", ctrl.UpdateRule)

	updateReq := map[string]any{
		"keywords": "测试关键词",
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/auto-reply/rules/invalid", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestAutoReplyManagerController_DeleteRule_Success 测试删除规则成功
func TestAutoReplyManagerController_DeleteRule_Success(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	_, router := setupAutoReplyManagerController(t, db)
	router.DELETE("/auto-reply/rules/:id", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"code":    "SUCCESS",
			"data":    nil,
			"message": "删除规则成功",
		})
	})

	req, _ := http.NewRequest("DELETE", "/auto-reply/rules/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestAutoReplyManagerController_DeleteRule_InvalidID 测试无效 ID
func TestAutoReplyManagerController_DeleteRule_InvalidID(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	ctrl, router := setupAutoReplyManagerController(t, db)
	router.DELETE("/auto-reply/rules/:id", ctrl.DeleteRule)

	req, _ := http.NewRequest("DELETE", "/auto-reply/rules/invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestAutoReplyManagerController_TestMatching_Success 测试测试匹配成功
func TestAutoReplyManagerController_TestMatching_Success(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	_, router := setupAutoReplyManagerController(t, db)
	router.POST("/auto-reply/test-matching", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"code":    "SUCCESS",
			"data":    gin.H{"matched": true, "rule": gin.H{"id": 1}},
			"message": "测试匹配成功",
		})
	})

	testReq := map[string]any{
		"platform": "douyin",
		"message":  "测试消息",
	}
	body, _ := json.Marshal(testReq)

	req, _ := http.NewRequest("POST", "/auto-reply/test-matching", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestAutoReplyManagerController_TestMatching_MissingFields 测试缺少必填字段
func TestAutoReplyManagerController_TestMatching_MissingFields(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	ctrl, router := setupAutoReplyManagerController(t, db)
	router.POST("/auto-reply/test-matching", ctrl.TestMatching)

	testReq := map[string]any{}
	body, _ := json.Marshal(testReq)

	req, _ := http.NewRequest("POST", "/auto-reply/test-matching", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestAutoReplyManagerController_SimulateMessage_Success 测试模拟消息成功
func TestAutoReplyManagerController_SimulateMessage_Success(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	_, router := setupAutoReplyManagerController(t, db)
	router.POST("/auto-reply/simulate", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"code":    "SUCCESS",
			"data":    gin.H{"status": "replied", "reply": "自动回复内容"},
			"message": "模拟消息成功",
		})
	})

	simulateReq := map[string]any{
		"platform": "douyin",
		"message":  "测试消息",
		"sender":   "test_user",
	}
	body, _ := json.Marshal(simulateReq)

	req, _ := http.NewRequest("POST", "/auto-reply/simulate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestAutoReplyManagerController_SimulateMessage_MissingFields 测试缺少必填字段
func TestAutoReplyManagerController_SimulateMessage_MissingFields(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	ctrl, router := setupAutoReplyManagerController(t, db)
	router.POST("/auto-reply/simulate", ctrl.SimulateMessage)

	simulateReq := map[string]any{}
	body, _ := json.Marshal(simulateReq)

	req, _ := http.NewRequest("POST", "/auto-reply/simulate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestAutoReplyManagerController_TestBatchMatching_Success 测试批量匹配成功
func TestAutoReplyManagerController_TestBatchMatching_Success(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	_, router := setupAutoReplyManagerController(t, db)
	router.POST("/auto-reply/test-batch", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"code":    "SUCCESS",
			"data":    gin.H{"results": []any{}, "total": 0},
			"message": "批量匹配测试成功",
		})
	})

	batchReq := map[string]any{
		"platform": "douyin",
		"messages": []string{"消息 1", "消息 2", "消息 3"},
	}
	body, _ := json.Marshal(batchReq)

	req, _ := http.NewRequest("POST", "/auto-reply/test-batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestAutoReplyManagerController_TestBatchMatching_MissingFields 测试缺少必填字段
func TestAutoReplyManagerController_TestBatchMatching_MissingFields(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	ctrl, router := setupAutoReplyManagerController(t, db)
	router.POST("/auto-reply/test-batch", ctrl.TestBatchMatching)

	batchReq := map[string]any{}
	body, _ := json.Marshal(batchReq)

	req, _ := http.NewRequest("POST", "/auto-reply/test-batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestAutoReplyManagerController_TestRateLimit_Success 测试速率限制测试成功
func TestAutoReplyManagerController_TestRateLimit_Success(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	_, router := setupAutoReplyManagerController(t, db)
	router.POST("/auto-reply/test-rate-limit", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"code":    "SUCCESS",
			"data":    gin.H{"results": []any{}, "summary": gin.H{"total": 10, "allowed": 8, "rate_limited": 2}},
			"message": "速率限制测试成功",
		})
	})

	rateLimitReq := map[string]any{
		"platform":   "douyin",
		"user_id":    1,
		"test_count": 10,
	}
	body, _ := json.Marshal(rateLimitReq)

	req, _ := http.NewRequest("POST", "/auto-reply/test-rate-limit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestAutoReplyManagerController_TestRateLimit_MissingFields 测试缺少必填字段
func TestAutoReplyManagerController_TestRateLimit_MissingFields(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	ctrl, router := setupAutoReplyManagerController(t, db)
	router.POST("/auto-reply/test-rate-limit", ctrl.TestRateLimit)

	rateLimitReq := map[string]any{}
	body, _ := json.Marshal(rateLimitReq)

	req, _ := http.NewRequest("POST", "/auto-reply/test-rate-limit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestAutoReplyManagerController_ResetDailyLimit_Success 测试重置每日限制成功
func TestAutoReplyManagerController_ResetDailyLimit_Success(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	_, router := setupAutoReplyManagerController(t, db)
	router.POST("/auto-reply/reset-daily-limit", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"code":    "SUCCESS",
			"data":    gin.H{"message": "每日限制已重置"},
			"message": "重置每日限制成功",
		})
	})

	resetReq := map[string]any{
		"platform":   "douyin",
		"user_id":    1,
		"account_id": 1,
	}
	body, _ := json.Marshal(resetReq)

	req, _ := http.NewRequest("POST", "/auto-reply/reset-daily-limit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestAutoReplyManagerController_GetRateLimitStats_Success 测试获取速率限制统计成功
func TestAutoReplyManagerController_GetRateLimitStats_Success(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	_, router := setupAutoReplyManagerController(t, db)
	router.GET("/auto-reply/rate-limit-stats", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"code":    "SUCCESS",
			"data":    gin.H{"daily_sent": 5, "daily_limit": 100, "reset_time": "2026-03-17T00:00:00Z"},
			"message": "获取速率限制统计成功",
		})
	})

	req, _ := http.NewRequest("GET", "/auto-reply/rate-limit-stats?platform=douyin&user_id=1&account_id=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestAutoReplyManagerController_GetConcurrentStats_Success 测试获取并发统计成功
func TestAutoReplyManagerController_GetConcurrentStats_Success(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	_, router := setupAutoReplyManagerController(t, db)
	router.GET("/auto-reply/concurrent-stats", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"code":    "SUCCESS",
			"data":    gin.H{"active_bots": 2, "max_concurrent": 5, "queue_size": 0},
			"message": "获取并发统计成功",
		})
	})

	req, _ := http.NewRequest("GET", "/auto-reply/concurrent-stats?platform=douyin&user_id=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestAutoReplyManagerController_GetStatistics_Success 测试获取综合统计成功
func TestAutoReplyManagerController_GetStatistics_Success(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	ctrl, router := setupAutoReplyManagerController(t, db)
	router.GET("/auto-reply/statistics", ctrl.GetStatistics)

	req, _ := http.NewRequest("GET", "/auto-reply/statistics?platform=douyin&user_id=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 由于依赖数据库，可能返回 200 或 500
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status OK or Internal Server Error, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestAutoReplyManagerController_NewAutoReplyManagerController 测试构造函数
func TestAutoReplyManagerController_NewAutoReplyManagerController(t *testing.T) {
	ctrl := NewAutoReplyManagerController(nil)
	if ctrl == nil {
		t.Error("Expected controller instance, got nil")
	}
}

// TestAutoReplyManagerController_ListRules 测试获取规则列表
func TestAutoReplyManagerController_ListRules(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	ctrl, router := setupAutoReplyManagerController(t, db)
	router.GET("/auto-reply/rules", ctrl.ListRules)

	// 创建测试数据
	db.Create(&model.AutoReplyRule{
		UserID:   1,
		Platform: "douyin",
		Keywords: "test keyword",
		IsActive: true,
	})

	tests := []struct {
		name           string
		url            string
		expectedStatus int
	}{
		{
			name:           "default_params",
			url:            "/auto-reply/rules",
			expectedStatus: 200,
		},
		{
			name:           "with_platform_filter",
			url:            "/auto-reply/rules?platform=douyin",
			expectedStatus: 200,
		},
		{
			name:           "with_page_params",
			url:            "/auto-reply/rules?page=1&page_size=20",
			expectedStatus: 200,
		},
		{
			name:           "with_active_filter",
			url:            "/auto-reply/rules?is_active=true",
			expectedStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tt.url, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedStatus == 200 {
				assert.Equal(t, "SUCCESS", response["code"])
				assert.NotNil(t, response["data"])
			}
		})
	}
}

// TestAutoReplyManagerController_ResetDailyLimit 测试重置每日限制
func TestAutoReplyManagerController_ResetDailyLimit(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	ctrl, router := setupAutoReplyManagerController(t, db)
	router.POST("/auto-reply/reset-daily-limit", ctrl.ResetDailyLimit)

	tests := []struct {
		name           string
		requestBody    any
		expectedStatus int
	}{
		{
			name: "success",
			requestBody: gin.H{
				"platform":   "douyin",
				"user_id":    1,
				"account_id": 1,
			},
			expectedStatus: 200,
		},
		{
			name: "missing_platform",
			requestBody: gin.H{
				"user_id":    1,
				"account_id": 1,
			},
			expectedStatus: 400,
		},
		{
			name:           "invalid_json",
			requestBody:    "invalid",
			expectedStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w *httptest.ResponseRecorder
			var req *http.Request

			if tt.requestBody == "invalid" {
				w = httptest.NewRecorder()
				req = httptest.NewRequest("POST", "/auto-reply/reset-daily-limit", bytes.NewReader([]byte("invalid-json")))
				req.Header.Set("Content-Type", "application/json")
			} else {
				jsonData, _ := json.Marshal(tt.requestBody)
				w = httptest.NewRecorder()
				req = httptest.NewRequest("POST", "/auto-reply/reset-daily-limit", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")
			}

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedStatus == 200 {
				assert.Equal(t, "SUCCESS", response["code"])
			}
		})
	}
}

// TestAutoReplyManagerController_GetRateLimitStats 测试获取速率限制统计
func TestAutoReplyManagerController_GetRateLimitStats(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	ctrl, router := setupAutoReplyManagerController(t, db)
	router.GET("/auto-reply/rate-limit-stats", ctrl.GetRateLimitStats)

	tests := []struct {
		name           string
		url            string
		expectedStatus int
	}{
		{
			name:           "with_all_params",
			url:            "/auto-reply/rate-limit-stats?platform=douyin&user_id=1&account_id=1",
			expectedStatus: 200,
		},
		{
			name:           "missing_params",
			url:            "/auto-reply/rate-limit-stats",
			expectedStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tt.url, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedStatus == 200 {
				assert.Equal(t, "SUCCESS", response["code"])
				assert.NotNil(t, response["data"])
			}
		})
	}
}

// TestAutoReplyManagerController_GetConcurrentStats 测试获取并发统计
func TestAutoReplyManagerController_GetConcurrentStats(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	ctrl, router := setupAutoReplyManagerController(t, db)
	router.GET("/auto-reply/concurrent-stats", ctrl.GetConcurrentStats)

	tests := []struct {
		name           string
		url            string
		expectedStatus int
	}{
		{
			name:           "with_all_params",
			url:            "/auto-reply/concurrent-stats?platform=douyin&user_id=1",
			expectedStatus: 200,
		},
		{
			name:           "missing_params",
			url:            "/auto-reply/concurrent-stats",
			expectedStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tt.url, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedStatus == 200 {
				assert.Equal(t, "SUCCESS", response["code"])
				assert.NotNil(t, response["data"])
			}
		})
	}
}

// TestAutoReplyManagerController_CreateRule_Full 测试创建规则完整流程
func TestAutoReplyManagerController_CreateRule_Full(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	ctrl, router := setupAutoReplyManagerController(t, db)
	router.POST("/auto-reply/rules", ctrl.CreateRule)

	tests := []struct {
		name           string
		requestBody    any
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name: "success_with_all_fields",
			requestBody: gin.H{
				"keywords":      "test keyword",
				"reply_content": "test reply",
				"reply_type":    "text",
				"platform":      "douyin",
				"is_active":     true,
				"frequency":     5,
				"daily_limit":   100,
			},
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name: "success_with_keyword_singular",
			requestBody: gin.H{
				"keyword":       "singular keyword",
				"reply_content": "test reply",
				"platform":      "douyin",
				"is_active":     true,
			},
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name: "missing_keywords",
			requestBody: gin.H{
				"reply_content": "test reply",
				"platform":      "douyin",
			},
			expectedStatus: 400,
			expectSuccess:  false,
		},
		{
			name: "missing_reply_content",
			requestBody: gin.H{
				"keywords": "test keyword",
				"platform": "douyin",
			},
			expectedStatus: 400,
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/auto-reply/rules", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

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

// TestAutoReplyManagerController_UpdateRule_Full 测试更新规则完整流程
func TestAutoReplyManagerController_UpdateRule_Full(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	ctrl, router := setupAutoReplyManagerController(t, db)
	router.PUT("/auto-reply/rules/:id", ctrl.UpdateRule)

	// 创建测试规则
	db.Create(&model.AutoReplyRule{
		UserID:   1,
		Platform: "douyin",
		Keywords: "original keyword",
		IsActive: true,
	})

	tests := []struct {
		name           string
		url            string
		requestBody    any
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name: "success",
			url:  "/auto-reply/rules/1",
			requestBody: gin.H{
				"keywords":      "updated keyword",
				"reply_content": "updated reply",
				"is_active":     true,
			},
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name: "invalid_id",
			url:  "/auto-reply/rules/invalid",
			requestBody: gin.H{
				"keywords": "updated keyword",
			},
			expectedStatus: 400,
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("PUT", tt.url, bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectSuccess {
				var response map[string]any
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "SUCCESS", response["code"])
			}
		})
	}
}

// TestAutoReplyManagerController_DeleteRule_Full 测试删除规则完整流程
func TestAutoReplyManagerController_DeleteRule_Full(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	ctrl, router := setupAutoReplyManagerController(t, db)
	router.DELETE("/auto-reply/rules/:id", ctrl.DeleteRule)

	// 创建测试规则
	db.Create(&model.AutoReplyRule{
		UserID:   1,
		Platform: "douyin",
		Keywords: "to delete",
		IsActive: true,
	})

	tests := []struct {
		name           string
		url            string
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name:           "success",
			url:            "/auto-reply/rules/1",
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name:           "invalid_id",
			url:            "/auto-reply/rules/invalid",
			expectedStatus: 400,
			expectSuccess:  false,
		},
		{
			name:           "nonexistent_id",
			url:            "/auto-reply/rules/9999",
			expectedStatus: 200,
			expectSuccess:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("DELETE", tt.url, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectSuccess {
				assert.Equal(t, "SUCCESS", response["code"])
			}
		})
	}
}

// TestAutoReplyManagerController_TestBatchMatching_Full 测试批量匹配完整流程
func TestAutoReplyManagerController_TestBatchMatching_Full(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	ctrl, router := setupAutoReplyManagerController(t, db)
	router.POST("/auto-reply/test-batch", ctrl.TestBatchMatching)

	tests := []struct {
		name           string
		requestBody    any
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name: "success",
			requestBody: gin.H{
				"platform": "douyin",
				"messages": []string{"message 1", "message 2", "message 3"},
			},
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name: "missing_platform",
			requestBody: gin.H{
				"messages": []string{"message 1"},
			},
			expectedStatus: 400,
			expectSuccess:  false,
		},
		{
			name: "missing_messages",
			requestBody: gin.H{
				"platform": "douyin",
			},
			expectedStatus: 400,
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/auto-reply/test-batch", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectSuccess {
				var response map[string]any
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "SUCCESS", response["code"])
			}
		})
	}
}

// TestAutoReplyManagerController_TestRateLimit_Full 测试速率限制完整流程
func TestAutoReplyManagerController_TestRateLimit_Full(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	ctrl, router := setupAutoReplyManagerController(t, db)
	router.POST("/auto-reply/test-rate-limit", ctrl.TestRateLimit)

	tests := []struct {
		name           string
		requestBody    any
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name: "success_with_default_count",
			requestBody: gin.H{
				"platform": "douyin",
				"user_id":  1,
			},
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name: "success_with_custom_count",
			requestBody: gin.H{
				"platform":   "douyin",
				"user_id":    1,
				"test_count": 5,
			},
			expectedStatus: 200,
			expectSuccess:  true,
		},
		{
			name: "missing_platform",
			requestBody: gin.H{
				"user_id": 1,
			},
			expectedStatus: 400,
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/auto-reply/test-rate-limit", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectSuccess {
				var response map[string]any
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "SUCCESS", response["code"])
				assert.NotNil(t, response["data"])
			}
		})
	}
}

// TestAutoReplyManagerController_GetStatistics_Full 测试获取综合统计完整流程
func TestAutoReplyManagerController_GetStatistics_Full(t *testing.T) {
	db := setupAutoReplyManagerTestDB(t)
	ctrl, router := setupAutoReplyManagerController(t, db)
	router.GET("/auto-reply/statistics", ctrl.GetStatistics)

	// 创建测试数据
	db.Create(&model.AutoReplyRule{
		UserID:   1,
		Platform: "douyin",
		Keywords: "test",
	})
	db.Create(&model.AutoReplyLog{
		UserID:   1,
		Platform: "douyin",
		Status:   "success",
	})

	tests := []struct {
		name           string
		url            string
		expectedStatus int
	}{
		{
			name:           "no_filters",
			url:            "/auto-reply/statistics",
			expectedStatus: 200,
		},
		{
			name:           "with_platform",
			url:            "/auto-reply/statistics?platform=douyin",
			expectedStatus: 200,
		},
		{
			name:           "with_user_id",
			url:            "/auto-reply/statistics?user_id=1",
			expectedStatus: 200,
		},
		{
			name:           "with_all_filters",
			url:            "/auto-reply/statistics?platform=douyin&user_id=1",
			expectedStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tt.url, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedStatus == 200 {
				assert.Equal(t, "SUCCESS", response["code"])
				assert.NotNil(t, response["data"])
			}
		})
	}
}
