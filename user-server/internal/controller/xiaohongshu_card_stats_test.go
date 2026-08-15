package controller

import (
	"context"
	"encoding/json"
	"hivemtk-user/internal/dto"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// MockXiaohongshuCardStatsService 模拟小红书卡片统计服务
type MockXiaohongshuCardStatsService struct {
	cardStats      map[uint]*dto.XiaohongshuCardStatsResponse
	overallStats   *dto.XiaohongshuCardOverallStatsResponse
	recordActivity func(ctx context.Context, cardID uint, userID uint, action string, username, ipAddress, userAgent string) error
}

func NewMockXiaohongshuCardStatsService() *MockXiaohongshuCardStatsService {
	return &MockXiaohongshuCardStatsService{
		cardStats: make(map[uint]*dto.XiaohongshuCardStatsResponse),
		overallStats: &dto.XiaohongshuCardOverallStatsResponse{
			TotalCards:     10,
			ActiveCards:    8,
			TotalViews:     1000,
			PopularCards:   []dto.PopularCard{},
			DailyStats:     []dto.DailyStat{},
			RecentActivity: []dto.Activity{},
		},
	}
}

func (m *MockXiaohongshuCardStatsService) GetCardStats(ctx context.Context, req *dto.XiaohongshuCardStatsRequest) (*dto.XiaohongshuCardStatsResponse, error) {
	if resp, ok := m.cardStats[req.CardID]; ok {
		return resp, nil
	}
	return &dto.XiaohongshuCardStatsResponse{
		CardID:    req.CardID,
		Title:     "测试卡片",
		ViewCount: 100,
		DailyStats: []dto.DailyStat{
			{Date: "2024-01-01", View: 50},
			{Date: "2024-01-02", View: 30},
			{Date: "2024-01-03", View: 20},
		},
		RecentActivity: []dto.Activity{
			{ID: 1, CardID: req.CardID, Action: "view", Username: "user1", IPAddress: "192.168.1.1", CreatedAt: time.Now().Format(time.RFC3339)},
		},
	}, nil
}

func (m *MockXiaohongshuCardStatsService) GetOverallStats(ctx context.Context, req *dto.XiaohongshuCardOverallStatsRequest) (*dto.XiaohongshuCardOverallStatsResponse, error) {
	return m.overallStats, nil
}

func (m *MockXiaohongshuCardStatsService) RecordActivity(ctx context.Context, cardID uint, userID uint, action string, username, ipAddress, userAgent string) error {
	if m.recordActivity != nil {
		return m.recordActivity(ctx, cardID, userID, action, username, ipAddress, userAgent)
	}
	return nil
}

func setupXiaohongshuCardStatsController(t *testing.T) (*XiaohongshuCardStatsController, *gin.Engine) {
	gin.SetMode(gin.TestMode)

	mockService := NewMockXiaohongshuCardStatsService()
	controller := NewXiaohongshuCardStatsController(mockService)

	router := gin.New()
	router.GET("/api/xiaohongshu/card/:id/stats", controller.GetCardStats)
	router.GET("/api/xiaohongshu/card/stats", controller.GetOverallStats)

	return controller, router
}

// TestNewXiaohongshuCardStatsController 测试创建控制器
func TestNewXiaohongshuCardStatsController(t *testing.T) {
	mockService := NewMockXiaohongshuCardStatsService()
	controller := NewXiaohongshuCardStatsController(mockService)

	assert.NotNil(t, controller)
	assert.NotNil(t, controller.statsService)
	assert.IsType(t, &MockXiaohongshuCardStatsService{}, controller.statsService)
}

// TestXiaohongshuCardStatsController_GetCardStats 测试获取卡片统计数据
func TestXiaohongshuCardStatsController_GetCardStats(t *testing.T) {
	_, router := setupXiaohongshuCardStatsController(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/xiaohongshu/card/1/stats", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "SUCCESS", response["code"]) 
	assert.NotNil(t, response["data"])
}

// TestXiaohongshuCardStatsController_GetCardStats_InvalidID 测试无效卡片 ID
func TestXiaohongshuCardStatsController_GetCardStats_InvalidID(t *testing.T) {
	_, router := setupXiaohongshuCardStatsController(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/xiaohongshu/card/invalid/stats", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.NotEqual(t, "SUCCESS", response["code"]) 
}

// TestXiaohongshuCardStatsController_GetCardStats_WithDateRange 测试带日期范围的请求
func TestXiaohongshuCardStatsController_GetCardStats_WithDateRange(t *testing.T) {
	_, router := setupXiaohongshuCardStatsController(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/xiaohongshu/card/1/stats?start_date=2024-01-01&end_date=2024-01-31&group_by=day", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "SUCCESS", response["code"])
	assert.NotNil(t, response["data"])
}

// TestXiaohongshuCardStatsController_GetCardStats_WithGroupBy 测试不同分组方式
func TestXiaohongshuCardStatsController_GetCardStats_WithGroupBy(t *testing.T) {
	_, router := setupXiaohongshuCardStatsController(t)

	testCases := []struct {
		groupBy string
	}{
		{"day"},
		{"week"},
		{"month"},
	}

	for _, tc := range testCases {
		t.Run("group_by_"+tc.groupBy, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/api/xiaohongshu/card/1/stats?group_by="+tc.groupBy, nil)
			router.ServeHTTP(w, req)

			assert.Equal(t, 200, w.Code)
		})
	}
}

// TestXiaohongshuCardStatsController_GetOverallStats 测试获取总体统计数据
func TestXiaohongshuCardStatsController_GetOverallStats(t *testing.T) {
	_, router := setupXiaohongshuCardStatsController(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/xiaohongshu/card/stats", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "SUCCESS", response["code"]) 

	data, ok := response["data"].(map[string]any)
	assert.True(t, ok)
	assert.Contains(t, data, "totalCards")
	assert.Contains(t, data, "activeCards")
	assert.Contains(t, data, "totalViews")
}

// TestXiaohongshuCardStatsController_GetOverallStats_WithDateRange 测试带日期范围的总体统计
func TestXiaohongshuCardStatsController_GetOverallStats_WithDateRange(t *testing.T) {
	_, router := setupXiaohongshuCardStatsController(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/xiaohongshu/card/stats?start_date=2024-01-01&end_date=2024-01-31&group_by=day", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "SUCCESS", response["code"])
}

// TestXiaohongshuCardStatsController_GetOverallStats_WithGroupBy 测试不同分组方式的总体统计
func TestXiaohongshuCardStatsController_GetOverallStats_WithGroupBy(t *testing.T) {
	_, router := setupXiaohongshuCardStatsController(t)

	testCases := []struct {
		groupBy string
	}{
		{"day"},
		{"week"},
		{"month"},
	}

	for _, tc := range testCases {
		t.Run("group_by_"+tc.groupBy, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/api/xiaohongshu/card/stats?group_by="+tc.groupBy, nil)
			router.ServeHTTP(w, req)

			assert.Equal(t, 200, w.Code)
		})
	}
}

// TestXiaohongshuCardStatsController_GetCardStats_DefaultDateRange 测试默认日期范围（最近 7 天）
func TestXiaohongshuCardStatsController_GetCardStats_DefaultDateRange(t *testing.T) {
	_, router := setupXiaohongshuCardStatsController(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/xiaohongshu/card/1/stats", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
}

// TestXiaohongshuCardStatsController_GetOverallStats_DefaultDateRange 测试默认日期范围（最近 30 天）
func TestXiaohongshuCardStatsController_GetOverallStats_DefaultDateRange(t *testing.T) {
	_, router := setupXiaohongshuCardStatsController(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/xiaohongshu/card/stats", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
}

// TestXiaohongshuCardStatsController_GetCardStats_LargeCardID 测试大卡片 ID
func TestXiaohongshuCardStatsController_GetCardStats_LargeCardID(t *testing.T) {
	_, router := setupXiaohongshuCardStatsController(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/xiaohongshu/card/999999/stats", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
}

// TestXiaohongshuCardStatsController_GetCardStats_NegativeCardID 测试负数卡片 ID
func TestXiaohongshuCardStatsController_GetCardStats_NegativeCardID(t *testing.T) {
	_, router := setupXiaohongshuCardStatsController(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/xiaohongshu/card/-1/stats", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)
}

// TestXiaohongshuCardStatsController_GetCardStats_ZeroCardID 测试零卡片 ID
func TestXiaohongshuCardStatsController_GetCardStats_ZeroCardID(t *testing.T) {
	_, router := setupXiaohongshuCardStatsController(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/xiaohongshu/card/0/stats", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
}

// TestXiaohongshuCardStatsController_GetOverallStats_EmptyResponse 测试空响应
func TestXiaohongshuCardStatsController_GetOverallStats_EmptyResponse(t *testing.T) {
	mockService := NewMockXiaohongshuCardStatsService()
	mockService.overallStats = &dto.XiaohongshuCardOverallStatsResponse{
		TotalCards:     0,
		ActiveCards:    0,
		TotalViews:     0,
		PopularCards:   []dto.PopularCard{},
		DailyStats:     []dto.DailyStat{},
		RecentActivity: []dto.Activity{},
	}

	controller := NewXiaohongshuCardStatsController(mockService)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/xiaohongshu/card/stats", controller.GetOverallStats)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/xiaohongshu/card/stats", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "SUCCESS", response["code"])
}

