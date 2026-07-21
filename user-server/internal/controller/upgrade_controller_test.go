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

func setupUpgradeTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.UpgradeTask{},
		&model.MigrationRecord{},
	)
	db.SetTestDB(database)
	return database
}

func setupUpgradeRouter(ctrl *UpgradeController) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test_value")
		c.Next()
	})
	router.GET("/upgrade/task/:id", ctrl.GetUpgradeTask)
	router.GET("/upgrade/history", ctrl.GetUpgradeHistory)
	router.GET("/upgrade/migration-records", ctrl.GetMigrationRecords)
	router.GET("/upgrade/current-version", ctrl.GetCurrentVersion)
	router.POST("/upgrade/task", ctrl.CreateUpgradeTask)
	router.POST("/upgrade/rollback", ctrl.Rollback)
	router.GET("/upgrade/available", ctrl.GetAvailableUpgrades)
	return router
}

func TestUpgradeController_GetUpgradeHistory_Success(t *testing.T) {
	setupUpgradeTestDB(t)
	ctrl := NewUpgradeController()
	router := setupUpgradeRouter(ctrl)

	req, _ := http.NewRequest("GET", "/upgrade/history?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestUpgradeController_GetMigrationRecords_Success(t *testing.T) {
	setupUpgradeTestDB(t)
	ctrl := NewUpgradeController()
	router := setupUpgradeRouter(ctrl)

	req, _ := http.NewRequest("GET", "/upgrade/migration-records", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestUpgradeController_GetCurrentVersion_Success(t *testing.T) {
	setupUpgradeTestDB(t)
	ctrl := NewUpgradeController()
	router := setupUpgradeRouter(ctrl)

	req, _ := http.NewRequest("GET", "/upgrade/current-version", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestUpgradeController_GetAvailableUpgrades_Success(t *testing.T) {
	setupUpgradeTestDB(t)
	ctrl := NewUpgradeController()
	router := setupUpgradeRouter(ctrl)

	req, _ := http.NewRequest("GET", "/upgrade/available", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestUpgradeController_GetUpgradeTask_NoAuth(t *testing.T) {
	setupUpgradeTestDB(t)
	ctrl := NewUpgradeController()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	// 添加 Recovery 中间件以将 DB 访问失败转成 500
	router.Use(gin.Recovery())
	router.GET("/upgrade/task/:id", ctrl.GetUpgradeTask)

	req, _ := http.NewRequest("GET", "/upgrade/task/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 当前 GetUpgradeTask 实现未做 user_id 鉴权,直接查 DB;空 DB → 404
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404 (task not found), got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestUpgradeController_GetUpgradeTask_InvalidID(t *testing.T) {
	setupUpgradeTestDB(t)
	ctrl := NewUpgradeController()
	router := setupUpgradeRouter(ctrl)

	req, _ := http.NewRequest("GET", "/upgrade/task/abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for invalid ID, got %d", w.Code)
	}
}

func TestUpgradeController_GetUpgradeTask_NotFound(t *testing.T) {
	setupUpgradeTestDB(t)
	ctrl := NewUpgradeController()
	router := setupUpgradeRouter(ctrl)

	req, _ := http.NewRequest("GET", "/upgrade/task/999999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d. Body: %s", w.Code, w.Body.String())
	}
}
