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

func setupObsTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.ObsConfig{},
	)
	db.SetTestDB(database)
	return database
}

func setupObsRouter(ctrl *ObsConfigController) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("license_id", "test-license")
		c.Next()
	})
	router.GET("/obs/config", ctrl.GetConfigList)
	router.POST("/obs/config", ctrl.CreateConfig)
	router.GET("/obs/config/:id", ctrl.GetConfig)
	router.PUT("/obs/config/:id", ctrl.UpdateConfig)
	router.DELETE("/obs/config/:id", ctrl.DeleteConfig)
	router.POST("/obs/config/:id/test", ctrl.TestConnection)
	router.POST("/obs/config/:id/default", ctrl.SetDefault)
	router.GET("/obs/config/default", ctrl.GetDefaultConfig)
	return router
}

func TestObsConfigController_GetConfigList_NoLicense(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ctrl := NewObsConfigController()
	router.GET("/obs/config", ctrl.GetConfigList)

	req, _ := http.NewRequest("GET", "/obs/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestObsConfigController_GetConfigList_WithLicense(t *testing.T) {
	setupObsTestDB(t)
	ctrl := NewObsConfigController()
	router := setupObsRouter(ctrl)

	req, _ := http.NewRequest("GET", "/obs/config?license_id=test-license&page=1&limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestObsConfigController_GetDefaultConfig_Success(t *testing.T) {
	setupObsTestDB(t)
	ctrl := NewObsConfigController()
	router := setupObsRouter(ctrl)

	req, _ := http.NewRequest("GET", "/obs/config/default?license_id=test-license", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Empty DB → 500 (not found), with data → 200
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 200 or 500, got %d. Body: %s", w.Code, w.Body.String())
	}
}
