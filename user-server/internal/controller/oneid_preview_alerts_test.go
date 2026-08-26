package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func setupOneIDPreviewTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t, &model.Customer{})
	db.SetTestDB(database)
	return database
}

func setupAlertUnreadTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t, &model.AlertRule{}, &model.AlertHistory{})
	db.SetTestDB(database)
	return database
}

// TestCustomerOneIDController_PreviewMergeRules 测试 POST /api/oneid/merge-rules/preview
func TestCustomerOneIDController_PreviewMergeRules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database := setupOneIDPreviewTestDB(t)
	ctrl := NewCustomerOneIDController()
	router := gin.New()
	router.POST("/api/oneid/merge-rules/preview", ctrl.PreviewMergeRules)

	// 种子数据：c1/c2 同手机号（应产生 1 个候选对），c3 不同手机号
	customers := []*model.Customer{
		{ID: "cust-1", UnifiedID: "u1", Name: "A", Phone: "13800001111"},
		{ID: "cust-2", UnifiedID: "u2", Name: "B", Phone: "13800001111"},
		{ID: "cust-3", UnifiedID: "u3", Name: "C", Phone: "13900002222"},
	}
	for _, c := range customers {
		if err := database.Create(c).Error; err != nil {
			t.Fatalf("seed customer failed: %v", err)
		}
	}

	doPreview := func(body any) (*httptest.ResponseRecorder, map[string]any) {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/oneid/merge-rules/preview", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w, resp
	}

	t.Run("enabled_phone_rule_finds_one_pair", func(t *testing.T) {
		w, resp := doPreview([]map[string]any{
			{"name": "同手机号合并", "fields": []string{"phone"}, "threshold": 95, "enabled": true},
		})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "SUCCESS", resp["code"])
		data := resp["data"].(map[string]any)
		assert.Equal(t, float64(1), data["candidateCount"])
		samples := data["samples"].([]any)
		assert.Len(t, samples, 1)
		sample := samples[0].(map[string]any)
		assert.Equal(t, float64(95), sample["score"])
		assert.NotEqual(t, sample["from"], sample["to"])
	})

	t.Run("disabled_rule_no_candidates", func(t *testing.T) {
		_, resp := doPreview([]map[string]any{
			{"name": "同手机号合并", "fields": []string{"phone"}, "threshold": 95, "enabled": false},
		})
		assert.Equal(t, "SUCCESS", resp["code"])
		data := resp["data"].(map[string]any)
		assert.Equal(t, float64(0), data["candidateCount"])
	})

	t.Run("invalid_body_returns_400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/oneid/merge-rules/preview", bytes.NewReader([]byte("not-json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// TestAlertRuleController_Unread 测试 GET /api/monitor/alerts/unread
func TestAlertRuleController_Unread(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database := setupAlertUnreadTestDB(t)
	ctrl := NewAlertRuleController()
	router := gin.New()
	router.GET("/api/monitor/alerts/unread", ctrl.Unread)

	now := time.Now()
	histories := []*model.AlertHistory{
		{RuleID: 1, RuleName: "CPU 过高", Source: "system", Severity: model.AlertSeverityWarning,
			Status: model.AlertHistoryFiring, TriggeredAt: now},
		{RuleID: 2, RuleName: "队列积压", Source: "reach", Severity: model.AlertSeverityCritical,
			Status: model.AlertHistoryFiring, TriggeredAt: now.Add(-time.Hour)},
		{RuleID: 3, RuleName: "已恢复", Source: "system", Severity: model.AlertSeverityInfo,
			Status: model.AlertHistoryResolved, TriggeredAt: now.Add(-2 * time.Hour)},
	}
	for _, h := range histories {
		if err := database.Create(h).Error; err != nil {
			t.Fatalf("seed alert history failed: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/monitor/alerts/unread", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "SUCCESS", resp["code"])

	data := resp["data"].(map[string]any)
	assert.Equal(t, float64(2), data["count"], "只统计 firing 状态告警")
	assert.Equal(t, float64(2), data["unread_count"], "unread_count 与 count 一致")
	list := data["list"].([]any)
	assert.Len(t, list, 2, "列表只含 firing 告警")
	first := list[0].(map[string]any)
	assert.Equal(t, "CPU 过高", first["rule_name"], "按触发时间倒序")
}
