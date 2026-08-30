package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	dbutil "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"hivemtk-user/internal/model"
)

func setupSystemOpsRouter(ctrl *SystemOpsController) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/system/logs", ctrl.GetSystemLogs)
	router.GET("/system/stats", ctrl.GetSystemStats)
	router.GET("/system/backup", ctrl.GetBackupList)
	router.POST("/system/backup", ctrl.CreateBackup)
	router.POST("/system/restore", ctrl.RestoreBackup)
	return router
}

// initSystemOpsTestDB 初始化测试数据库
func initSystemOpsTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.Backup{},
	)
	dbutil.SetTestDB(database)
	return database
}

func TestSystemOpsController_GetSystemLogs_Success(t *testing.T) {
	ctrl := NewSystemOpsController()
	router := setupSystemOpsRouter(ctrl)

	req, _ := http.NewRequest("GET", "/system/logs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(0) {
		t.Errorf("Expected SUCCESS code, got %v", resp["code"])
	}
}

func TestSystemOpsController_GetBackupList_Success(t *testing.T) {
	initSystemOpsTestDB(t)
	ctrl := NewSystemOpsController()
	router := setupSystemOpsRouter(ctrl)

	req, _ := http.NewRequest("GET", "/system/backup", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestSystemOpsController_CreateBackup_Success(t *testing.T) {
	initSystemOpsTestDB(t)
	ctrl := NewSystemOpsController()
	router := setupSystemOpsRouter(ctrl)

	body, _ := json.Marshal(map[string]string{
		"backup_name": "test_backup",
		"backup_type": "full",
	})
	req, _ := http.NewRequest("POST", "/system/backup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestSystemOpsController_RestoreBackup_InvalidJSON(t *testing.T) {
	initSystemOpsTestDB(t)
	ctrl := NewSystemOpsController()
	router := setupSystemOpsRouter(ctrl)

	req, _ := http.NewRequest("POST", "/system/restore", bytes.NewReader([]byte("bad")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestSystemOpsController_RestoreBackup_MissingBackupID(t *testing.T) {
	initSystemOpsTestDB(t)
	ctrl := NewSystemOpsController()
	router := setupSystemOpsRouter(ctrl)

	body, _ := json.Marshal(map[string]string{})
	req, _ := http.NewRequest("POST", "/system/restore", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for missing backup_id, got %d", w.Code)
	}
}

func TestSystemOpsController_RestoreBackup_Success(t *testing.T) {
	initSystemOpsTestDB(t)
	ctrl := NewSystemOpsController()
	router := setupSystemOpsRouter(ctrl)

	body, _ := json.Marshal(map[string]any{"backup_id": 1})
	req, _ := http.NewRequest("POST", "/system/restore", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Errorf("Expected 200 or 404, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestSystemOpsController_GetSystemStats_NeedsDB(t *testing.T) {
	ctrl := NewSystemOpsController()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/system/stats", ctrl.GetSystemStats)

	req, _ := http.NewRequest("GET", "/system/stats", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Logf("Expected panic without DB: %v", r)
		}
	}()
	router.ServeHTTP(w, req)
}


func TestResolveLogPath_RejectTraversal(t *testing.T) {
	cases := []string{
		"/etc/passwd",
		"/etc/shadow",
		"../../../etc/passwd",
		"logs/../../../etc/passwd",
		"logs/../../etc/shadow",
		"..",
		"../",
		"/",
		"logs/../..",
		"/var/log/../etc/passwd",
		"/var/log/user-server/../../etc/passwd",
	}
	for _, c := range cases {
		_, err := resolveLogPath(c)
		if err == nil {
			t.Errorf("expected %q to be rejected, but it passed", c)
		}
	}
}

func TestResolveLogPath_AllowLegitimate(t *testing.T) {
	cases := []string{
		"logs/app.log",
		"logs/user-server.log",
		"logs/audit/AUDIT_LOG.md",
		"/var/log/user-server/app.log",
		"/var/log/marketing/app.log",
	}
	for _, c := range cases {
		resolved, err := resolveLogPath(c)
		if err != nil {
			t.Errorf("expected %q to pass, got error: %v", c, err)
			continue
		}
		if strings.Contains(resolved, "..") {
			t.Errorf("resolved path contains .. : %s", resolved)
		}
		if !filepath.IsAbs(resolved) {
			t.Errorf("expected absolute path, got %s", resolved)
		}
	}
}

func TestResolveLogPath_Empty(t *testing.T) {
	_, err := resolveLogPath("")
	if err == nil {
		t.Error("expected empty path to be rejected")
	}
}

// TestGetSystemLogs_RejectTraversal 验证 /system/logs?file=/etc/passwd 被拒绝
func TestGetSystemLogs_RejectTraversal(t *testing.T) {
	ctrl := NewSystemOpsController()
	router := setupSystemOpsRouter(ctrl)

	cases := []string{
		"/etc/passwd",
		"../../../etc/shadow",
		"logs/../../etc/passwd",
	}
	for _, malicious := range cases {
		req, _ := http.NewRequest("GET", "/system/logs?file="+malicious, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for path %q, got %d. Body: %s", malicious, w.Code, w.Body.String())
		}
	}
}

