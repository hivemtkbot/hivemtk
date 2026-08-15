package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"

	"hivemtk-user/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupMigrationTestDB 设置迁移任务测试数据库
func setupMigrationTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.UpgradeTask{},
		&model.MigrationRecord{},
	)
	db.SetTestDB(database)
	return database
}

// setupMigrationRouter 设置迁移路由（路径已由 /upgrade/* 改为 /migration/*）
func setupMigrationRouter(ctrl *MigrationController) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test_value")
		c.Next()
	})
	router.GET("/migration/task/:id", ctrl.GetUpgradeTask)
	router.GET("/migration/history", ctrl.GetUpgradeHistory)
	router.GET("/migration/records", ctrl.GetMigrationRecords)
	router.GET("/migration/current-version", ctrl.GetCurrentVersion)
	router.POST("/migration/task", ctrl.CreateUpgradeTask)
	router.POST("/migration/rollback", ctrl.Rollback)
	router.GET("/migration/available", ctrl.GetAvailableUpgrades)
	return router
}

func TestMigrationController_GetUpgradeHistory_Success(t *testing.T) {
	setupMigrationTestDB(t)
	ctrl := NewMigrationController(nil)
	router := setupMigrationRouter(ctrl)

	req, _ := http.NewRequest("GET", "/migration/history?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestMigrationController_GetMigrationRecords_Success(t *testing.T) {
	setupMigrationTestDB(t)
	ctrl := NewMigrationController(nil)
	router := setupMigrationRouter(ctrl)

	req, _ := http.NewRequest("GET", "/migration/records", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestMigrationController_GetCurrentVersion_Success(t *testing.T) {
	setupMigrationTestDB(t)
	ctrl := NewMigrationController(nil)
	router := setupMigrationRouter(ctrl)

	req, _ := http.NewRequest("GET", "/migration/current-version", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestMigrationController_GetAvailableUpgrades_Success(t *testing.T) {
	setupMigrationTestDB(t)
	ctrl := NewMigrationController(nil)
	router := setupMigrationRouter(ctrl)

	req, _ := http.NewRequest("GET", "/migration/available", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestMigrationController_GetUpgradeTask_NoAuth(t *testing.T) {
	setupMigrationTestDB(t)
	ctrl := NewMigrationController(nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/migration/task/:id", ctrl.GetUpgradeTask)

	req, _ := http.NewRequest("GET", "/migration/task/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404 (task not found), got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestMigrationController_GetUpgradeTask_InvalidID(t *testing.T) {
	setupMigrationTestDB(t)
	ctrl := NewMigrationController(nil)
	router := setupMigrationRouter(ctrl)

	req, _ := http.NewRequest("GET", "/migration/task/abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for invalid ID, got %d", w.Code)
	}
}

func TestMigrationController_GetUpgradeTask_NotFound(t *testing.T) {
	setupMigrationTestDB(t)
	ctrl := NewMigrationController(nil)
	router := setupMigrationRouter(ctrl)

	req, _ := http.NewRequest("GET", "/migration/task/999999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d. Body: %s", w.Code, w.Body.String())
	}
}

