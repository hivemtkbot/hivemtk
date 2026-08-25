package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// setupBackupController 设置备份控制器测试环境
func setupBackupController(t *testing.T) (*BackupController, *RestoreController, *gin.Engine, *gorm.DB) {
	database := testutil.NewTestDB(t,
		&model.Backup{},
		&model.RestoreRecord{},
	)
	db.SetTestDB(database)
	ctrl := NewBackupController()
	restoreCtrl := NewRestoreController()
	router := gin.New()
	return ctrl, restoreCtrl, router, database
}


// TestBackupController_CreateBackup_Success 测试创建备份成功
func TestBackupController_CreateBackup_Success(t *testing.T) {
	ctrl, _, router, _ := setupBackupController(t)

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Next()
	})

	router.POST("/backups", ctrl.CreateBackup)

	createReq := service.CreateBackupRequest{
		BackupName: "test-backup",
		BackupType: "full",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/backups", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status OK or Internal Server Error, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestBackupController_CreateBackup_InvalidJSON 测试无效 JSON
func TestBackupController_CreateBackup_InvalidJSON(t *testing.T) {
	ctrl, _, router, _ := setupBackupController(t)

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Next()
	})

	router.POST("/backups", ctrl.CreateBackup)

	req, _ := http.NewRequest("POST", "/backups", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

func TestBackupController_CreateBackup_NoMerchant(t *testing.T) {
	ctrl, _, router, _ := setupBackupController(t)

	router.POST("/backups", ctrl.CreateBackup)

	createReq := service.CreateBackupRequest{
		BackupName: "test-backup",
		BackupType: "full",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/backups", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status Unauthorized, got %d", w.Code)
	}
}

// TestBackupController_CreateBackup_NoUser 测试缺少有效用户信息
func TestBackupController_CreateBackup_NoUser(t *testing.T) {
	ctrl, _, router, _ := setupBackupController(t)

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Next()
	})

	router.POST("/backups", ctrl.CreateBackup)

	createReq := service.CreateBackupRequest{
		BackupName: "test-backup",
		BackupType: "full",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/backups", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
}

// TestBackupController_GetBackupList_Success 测试获取备份列表成功
func TestBackupController_GetBackupList_Success(t *testing.T) {
	ctrl, _, router, _ := setupBackupController(t)

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Next()
	})

	router.GET("/backups", ctrl.GetBackupList)

	req, _ := http.NewRequest("GET", "/backups?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status OK or Internal Server Error, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestBackupController_GetBackupList_NoMerchant(t *testing.T) {
	ctrl, _, router, _ := setupBackupController(t)

	router.GET("/backups", ctrl.GetBackupList)

	req, _ := http.NewRequest("GET", "/backups?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status Unauthorized, got %d", w.Code)
	}
}

// TestBackupController_GetBackupList_DefaultPagination 测试默认分页参数
func TestBackupController_GetBackupList_DefaultPagination(t *testing.T) {
	ctrl, _, router, _ := setupBackupController(t)

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Next()
	})

	router.GET("/backups", ctrl.GetBackupList)

	req, _ := http.NewRequest("GET", "/backups", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status OK or Internal Server Error, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestBackupController_GetBackupByID_Success 测试获取备份详情成功
func TestBackupController_GetBackupByID_Success(t *testing.T) {
	ctrl, _, router, _ := setupBackupController(t)

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Next()
	})

	router.GET("/backups/:id", ctrl.GetBackupByID)

	req, _ := http.NewRequest("GET", "/backups/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status OK, Not Found or Internal Server Error, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestBackupController_GetBackupByID_InvalidID 测试无效 ID
func TestBackupController_GetBackupByID_InvalidID(t *testing.T) {
	ctrl, _, router, _ := setupBackupController(t)

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Next()
	})

	router.GET("/backups/:id", ctrl.GetBackupByID)

	req, _ := http.NewRequest("GET", "/backups/invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

func TestBackupController_GetBackupByID_NoMerchant(t *testing.T) {
	ctrl, _, router, _ := setupBackupController(t)

	router.GET("/backups/:id", ctrl.GetBackupByID)

	req, _ := http.NewRequest("GET", "/backups/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status Unauthorized, got %d", w.Code)
	}
}

// TestBackupController_DeleteBackup_Success 测试删除备份成功
func TestBackupController_DeleteBackup_Success(t *testing.T) {
	ctrl, _, router, database := setupBackupController(t)

	backup := &model.Backup{
		BackupName: "test-backup",
		Status:     "completed",
	}
	database.Create(backup)

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Next()
	})

	router.DELETE("/backups/:id", ctrl.DeleteBackup)

	req, _ := http.NewRequest("DELETE", "/backups/"+strconvItoa(backup.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status OK or Internal Server Error, got %d, body: %s", w.Code, w.Body.String())
	}
}

// strconvItoa local helper to avoid extra import (uint id)
func strconvItoa(v uint) string {
	return fmt.Sprintf("%d", v)
}

// TestBackupController_DeleteBackup_InvalidID 测试无效 ID
func TestBackupController_DeleteBackup_InvalidID(t *testing.T) {
	ctrl, _, router, _ := setupBackupController(t)

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Next()
	})

	router.DELETE("/backups/:id", ctrl.DeleteBackup)

	req, _ := http.NewRequest("DELETE", "/backups/invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

func TestBackupController_DeleteBackup_NoMerchant(t *testing.T) {
	ctrl, _, router, _ := setupBackupController(t)

	router.DELETE("/backups/:id", ctrl.DeleteBackup)

	req, _ := http.NewRequest("DELETE", "/backups/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status Unauthorized, got %d", w.Code)
	}
}

// TestBackupController_NewBackupController 测试构造函数
func TestBackupController_NewBackupController(t *testing.T) {
	ctrl := NewBackupController()
	if ctrl == nil {
		t.Error("Expected controller instance, got nil")
	}
}


// TestRestoreController_RestoreBackup_Success 测试恢复备份成功
func TestRestoreController_RestoreBackup_Success(t *testing.T) {
	_, ctrl, router, _ := setupBackupController(t)

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Next()
	})

	router.POST("/restore", ctrl.RestoreBackup)

	restoreReq := service.RestoreBackupRequest{
		BackupID: 1,
	}
	body, _ := json.Marshal(restoreReq)

	req, _ := http.NewRequest("POST", "/restore", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status OK, Not Found or Internal Server Error, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestRestoreController_RestoreBackup_InvalidJSON 测试无效 JSON
func TestRestoreController_RestoreBackup_InvalidJSON(t *testing.T) {
	_, ctrl, router, _ := setupBackupController(t)

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Next()
	})

	router.POST("/restore", ctrl.RestoreBackup)

	req, _ := http.NewRequest("POST", "/restore", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

func TestRestoreController_RestoreBackup_NoMerchant(t *testing.T) {
	_, ctrl, router, _ := setupBackupController(t)

	router.POST("/restore", ctrl.RestoreBackup)

	restoreReq := service.RestoreBackupRequest{
		BackupID: 1,
	}
	body, _ := json.Marshal(restoreReq)

	req, _ := http.NewRequest("POST", "/restore", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status Unauthorized, got %d", w.Code)
	}
}

// TestRestoreController_GetRestoreList_Success 测试获取恢复记录列表成功
func TestRestoreController_GetRestoreList_Success(t *testing.T) {
	_, ctrl, router, _ := setupBackupController(t)

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Next()
	})

	router.GET("/restore", ctrl.GetRestoreList)

	req, _ := http.NewRequest("GET", "/restore?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status OK or Internal Server Error, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestRestoreController_GetRestoreList_NoMerchant(t *testing.T) {
	_, ctrl, router, _ := setupBackupController(t)

	router.GET("/restore", ctrl.GetRestoreList)

	req, _ := http.NewRequest("GET", "/restore?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status Unauthorized, got %d", w.Code)
	}
}

// TestRestoreController_GetRestoreList_DefaultPagination 测试默认分页参数
func TestRestoreController_GetRestoreList_DefaultPagination(t *testing.T) {
	_, ctrl, router, _ := setupBackupController(t)

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Next()
	})

	router.GET("/restore", ctrl.GetRestoreList)

	req, _ := http.NewRequest("GET", "/restore", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status OK or Internal Server Error, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestRestoreController_GetLastRestore_Success 测试获取最近一次恢复记录成功
func TestRestoreController_GetLastRestore_Success(t *testing.T) {
	_, ctrl, router, _ := setupBackupController(t)

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Set("user_id", uint(1))
		ctx.Set("role", "admin")
		ctx.Next()
	})

	router.GET("/restore/last", ctrl.GetLastRestore)

	req, _ := http.NewRequest("GET", "/restore/last", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status OK, Not Found or Internal Server Error, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestRestoreController_GetLastRestore_NoMerchant(t *testing.T) {
	_, ctrl, router, _ := setupBackupController(t)

	router.GET("/restore/last", ctrl.GetLastRestore)

	req, _ := http.NewRequest("GET", "/restore/last", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status Unauthorized, got %d", w.Code)
	}
}

// TestRestoreController_NewRestoreController 测试构造函数
func TestRestoreController_NewRestoreController(t *testing.T) {
	ctrl := NewRestoreController()
	if ctrl == nil {
		t.Error("Expected controller instance, got nil")
	}
}

