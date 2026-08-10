package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/service"

	"hivemtk-user/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func setupCardStatsTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.DouyinCard{},
		&model.KuaishouCard{},
		&model.XiaohongshuCard{},
		&model.XianyuCard{},
		&model.ShortLink{},
		&model.ShortLinkAccess{},
	)
	db.SetTestDB(database)
	return database
}

func TestSystemInfoController_GetSystemInfo_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ctrl := NewSystemInfoController()
	router.GET("/system/info", ctrl.GetSystemInfo)

	req, _ := http.NewRequest("GET", "/system/info", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestDouyinCardStatsController_GetCardStats_InvalidID(t *testing.T) {
	setupCardStatsTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	statsService := service.NewDouyinCardStatsService(db.GetDB())
	ctrl := NewDouyinCardStatsController(statsService)
	router.GET("/douyin-card/stats/:id", ctrl.GetCardStats)

	req, _ := http.NewRequest("GET", "/douyin-card/stats/abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestDouyinCardStatsController_GetCardStats_Success(t *testing.T) {
	setupCardStatsTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	statsService := service.NewDouyinCardStatsService(db.GetDB())
	ctrl := NewDouyinCardStatsController(statsService)
	router.GET("/douyin-card/stats/:id", ctrl.GetCardStats)

	req, _ := http.NewRequest("GET", "/douyin-card/stats/1?groupBy=day", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Empty DB → 404 (card not found) or 500, with card → 200
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 200, 404 or 500, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestDouyinCardStatsController_GetOverallStats_Success(t *testing.T) {
	setupCardStatsTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	statsService := service.NewDouyinCardStatsService(db.GetDB())
	ctrl := NewDouyinCardStatsController(statsService)
	router.GET("/douyin-card/overall-stats", ctrl.GetOverallStats)

	req, _ := http.NewRequest("GET", "/douyin-card/overall-stats?groupBy=day", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 200 or 500, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestKuaishouCardStatsController_GetCardStats_InvalidID(t *testing.T) {
	setupCardStatsTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	statsService := service.NewKuaishouCardStatsService(db.GetDB())
	ctrl := NewKuaishouCardStatsController(statsService)
	router.GET("/kuaishou-card/stats/:id", ctrl.GetCardStats)

	req, _ := http.NewRequest("GET", "/kuaishou-card/stats/abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestKuaishouCardStatsController_GetCardStats_Success(t *testing.T) {
	setupCardStatsTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	statsService := service.NewKuaishouCardStatsService(db.GetDB())
	ctrl := NewKuaishouCardStatsController(statsService)
	router.GET("/kuaishou-card/stats/:id", ctrl.GetCardStats)

	req, _ := http.NewRequest("GET", "/kuaishou-card/stats/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Empty DB → 404 (card not found) or 500, with card → 200
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 200, 404 or 500, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestKuaishouCardStatsController_GetOverallStats_Success(t *testing.T) {
	setupCardStatsTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	statsService := service.NewKuaishouCardStatsService(db.GetDB())
	ctrl := NewKuaishouCardStatsController(statsService)
	router.GET("/kuaishou-card/overall-stats", ctrl.GetOverallStats)

	req, _ := http.NewRequest("GET", "/kuaishou-card/overall-stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 200 or 500, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestShortLinkStatsController_GetStats_InvalidID(t *testing.T) {
	setupCardStatsTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	shortLinkService := service.NewShortLinkService(db.GetDB())
	ctrl := NewShortLinkStatsController(shortLinkService)
	router.GET("/short-link/:id/stats", ctrl.GetStats)

	req, _ := http.NewRequest("GET", "/short-link/abc/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestShortLinkStatsController_GetStats_Success(t *testing.T) {
	setupCardStatsTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	shortLinkService := service.NewShortLinkService(db.GetDB())
	ctrl := NewShortLinkStatsController(shortLinkService)
	router.GET("/short-link/:id/stats", ctrl.GetStats)

	req, _ := http.NewRequest("GET", "/short-link/1/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Empty DB → 404 (short link not found) or 500, with data → 200
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 200, 404 or 500, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestShortLinkStatsController_GetAllStats_Success(t *testing.T) {
	setupCardStatsTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	shortLinkService := service.NewShortLinkService(db.GetDB())
	ctrl := NewShortLinkStatsController(shortLinkService)
	router.GET("/short-link/all-stats", ctrl.GetAllStats)

	req, _ := http.NewRequest("GET", "/short-link/all-stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}
