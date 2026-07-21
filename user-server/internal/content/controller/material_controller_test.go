package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"marketing/internal/content/dto"
	"marketing/internal/content/model"
	"marketing/internal/content/service"
	sysmodel "marketing/internal/model"
	"marketing/internal/pkg/utils/db"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// newMaterialControllerForTest 创建用于测试的 MaterialController
func newMaterialControllerForTest() *MaterialController {
	return &MaterialController{
		service: service.NewMaterialServiceWithDB(db.GetDB()),
	}
}

// setupMaterialTestDB 设置素材测试数据库
func setupMaterialTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&sysmodel.SystemUser{},
		&sysmodel.User{},
		&sysmodel.License{},
		&model.Material{},
		&model.MaterialCategory{},
	)
	db.SetTestDB(database)
	return database
}

// createTestLicense 创建测试许可证
func createTestLicense(t *testing.T, db *gorm.DB) *sysmodel.License {
	license := &sysmodel.License{
		Key:      "TESTLICENSEKEY001",
		Status:   "active",
		ExpireAt: time.Now().Add(365 * 24 * time.Hour),
	}
	db.Create(license)
	return license
}

// createTestCategory 创建测试分类
func createTestCategory(t *testing.T, db *gorm.DB, licenseID string) *model.MaterialCategory {
	category := &model.MaterialCategory{
		Name:          "测试分类",
		Type:          model.MaterialTypeImage,
		LicenseID:     licenseID,
		UserID:        "test-user",
		Status:        "active",
		MaterialCount: 0,
	}
	db.Create(category)
	return category
}

// createTestMaterial 创建测试素材
func createTestMaterial(t *testing.T, db *gorm.DB, licenseID string, categoryID string) *model.Material {
	material := &model.Material{
		Name:       "测试素材",
		Type:       model.MaterialTypeImage,
		CategoryID: categoryID,
		URL:        "https://example.com/test.jpg",
		Size:       102400,
		MimeType:   "image/jpeg",
		Hash:       "test-hash-001",
		Width:      800,
		Height:     600,
		LicenseID:  licenseID,
		UserID:     "test-user",
		UsageCount: 0,
		Status:     "active",
	}
	db.Create(material)
	return material
}

// setupContextWithLicense 设置带有许可证信息的上下文
func setupContextWithLicense(router *gin.Engine, path string, handler gin.HandlerFunc, licenseID string, userID string) {
	router.GET(path, func(c *gin.Context) {
		c.Set("license_id", licenseID)
		c.Set("user_id", userID)
		handler(c)
	})
}

// TestMaterialController_GetMaterialList_Success 测试获取素材列表成功
func TestMaterialController_GetMaterialList_Success(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)
	category := createTestCategory(t, database, license.ID)
	createTestMaterial(t, database, license.ID, category.ID)

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.GET("/materials", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.GetMaterialList(c)
	})

	req, _ := http.NewRequest("GET", "/materials?page=1&limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["code"] != "SUCCESS" {
		t.Errorf("Expected code SUCCESS, got %v", response["code"])
	}
}

// TestMaterialController_GetMaterialList_Unauthorized 测试未授权访问
func TestMaterialController_GetMaterialList_Unauthorized(t *testing.T) {
	setupMaterialTestDB(t)
	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()
	router.GET("/materials", ctrl.GetMaterialList)

	req, _ := http.NewRequest("GET", "/materials?page=1&limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status Unauthorized, got %d", w.Code)
	}
}

// TestMaterialController_GetMaterialList_WithFilters 测试带筛选条件的素材列表
func TestMaterialController_GetMaterialList_WithFilters(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)
	category := createTestCategory(t, database, license.ID)
	createTestMaterial(t, database, license.ID, category.ID)

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.GET("/materials", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.GetMaterialList(c)
	})

	// 测试 category_id 筛选
	req, _ := http.NewRequest("GET", "/materials?category_id="+category.ID+"&type=image", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestMaterialController_GetMaterialList_WithSearch 测试带搜索的素材列表
func TestMaterialController_GetMaterialList_WithSearch(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)
	category := createTestCategory(t, database, license.ID)
	createTestMaterial(t, database, license.ID, category.ID)

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.GET("/materials", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.GetMaterialList(c)
	})

	// 测试 search 参数
	req, _ := http.NewRequest("GET", "/materials?search=测试", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}

	// 测试 keyword 参数（兼容性）
	req, _ = http.NewRequest("GET", "/materials?keyword=测试", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK with keyword param, got %d", w.Code)
	}
}

// TestMaterialController_GetMaterialList_EmptyList 测试空素材列表
func TestMaterialController_GetMaterialList_EmptyList(t *testing.T) {
	setupMaterialTestDB(t)
	license := createTestLicense(t, db.GetDB())

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.GET("/materials", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.GetMaterialList(c)
	})

	req, _ := http.NewRequest("GET", "/materials", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestMaterialController_UpdateMaterialUsage_Success 测试更新素材使用次数成功
func TestMaterialController_UpdateMaterialUsage_Success(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)
	material := createTestMaterial(t, database, license.ID, "")

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.PUT("/materials/:id/usage", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.UpdateMaterialUsage(c)
	})

	req, _ := http.NewRequest("PUT", "/materials/"+material.ID+"/usage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestMaterialController_UpdateMaterialUsage_EmptyID 测试素材 ID 为空
func TestMaterialController_UpdateMaterialUsage_EmptyID(t *testing.T) {
	setupMaterialTestDB(t)
	license := createTestLicense(t, db.GetDB())

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.PUT("/materials/:id/usage", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.UpdateMaterialUsage(c)
	})

	req, _ := http.NewRequest("PUT", "/materials//usage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestMaterialController_UpdateMaterialUsage_Unauthorized 测试未授权访问
func TestMaterialController_UpdateMaterialUsage_Unauthorized(t *testing.T) {
	setupMaterialTestDB(t)
	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()
	router.PUT("/materials/:id/usage", ctrl.UpdateMaterialUsage)

	req, _ := http.NewRequest("PUT", "/materials/test-id/usage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status Unauthorized, got %d", w.Code)
	}
}

// TestMaterialController_UpdateMaterialUsage_NotFound 测试素材不存在
func TestMaterialController_UpdateMaterialUsage_NotFound(t *testing.T) {
	setupMaterialTestDB(t)
	license := createTestLicense(t, db.GetDB())

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.PUT("/materials/:id/usage", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.UpdateMaterialUsage(c)
	})

	req, _ := http.NewRequest("PUT", "/materials/non-existent-id/usage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status Not Found or Internal Server Error, got %d", w.Code)
	}
}

// TestMaterialController_GetMaterialStats_Success 测试获取素材统计信息成功
func TestMaterialController_GetMaterialStats_Success(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)
	category := createTestCategory(t, database, license.ID)
	createTestMaterial(t, database, license.ID, category.ID)

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.GET("/materials/stats", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.GetMaterialStats(c)
	})

	req, _ := http.NewRequest("GET", "/materials/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["code"] != "SUCCESS" {
		t.Errorf("Expected code SUCCESS, got %v", response["code"])
	}
}

// TestMaterialController_GetMaterialStats_Unauthorized 测试未授权访问
func TestMaterialController_GetMaterialStats_Unauthorized(t *testing.T) {
	setupMaterialTestDB(t)
	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()
	router.GET("/materials/stats", ctrl.GetMaterialStats)

	req, _ := http.NewRequest("GET", "/materials/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status Unauthorized, got %d", w.Code)
	}
}

// TestMaterialController_GetMaterialStats_EmptyLicense 测试许可证 ID 为空
func TestMaterialController_GetMaterialStats_EmptyLicense(t *testing.T) {
	setupMaterialTestDB(t)
	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.GET("/materials/stats", func(c *gin.Context) {
		c.Set("license_id", "")
		c.Set("user_id", "test-user")
		ctrl.GetMaterialStats(c)
	})

	req, _ := http.NewRequest("GET", "/materials/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status Unauthorized, got %d", w.Code)
	}
}

// TestMaterialController_UploadMaterial_Success 测试上传素材成功
func TestMaterialController_UploadMaterial_Success(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)
	createTestCategory(t, database, license.ID)

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.POST("/materials/upload", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.UploadMaterial(c)
	})

	// 创建 multipart/form-data 请求
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 添加文件
	fileWriter, err := writer.CreateFormFile("file", "test.jpg")
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}
	// 写入简单的 JPEG 文件头
	fileWriter.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0})

	// 添加 category_id 字段
	err = writer.WriteField("category_id", license.ID)
	if err != nil {
		t.Fatalf("Failed to write category_id: %v", err)
	}

	writer.Close()

	req, _ := http.NewRequest("POST", "/materials/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 由于 OBS 上传会失败，这里只验证前置验证通过
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status OK or Internal Server Error (OBS), got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestMaterialController_UploadMaterial_Unauthorized 测试未授权访问
func TestMaterialController_UploadMaterial_Unauthorized(t *testing.T) {
	setupMaterialTestDB(t)
	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()
	router.POST("/materials/upload", ctrl.UploadMaterial)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.Close()

	req, _ := http.NewRequest("POST", "/materials/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status Unauthorized, got %d", w.Code)
	}
}

// TestMaterialController_UploadMaterial_MissingCategory 测试缺少分类 ID
func TestMaterialController_UploadMaterial_MissingCategory(t *testing.T) {
	setupMaterialTestDB(t)
	license := createTestLicense(t, db.GetDB())

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.POST("/materials/upload", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.UploadMaterial(c)
	})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	fileWriter, _ := writer.CreateFormFile("file", "test.jpg")
	fileWriter.Write([]byte("test content"))

	writer.Close()

	req, _ := http.NewRequest("POST", "/materials/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestMaterialController_UploadMaterial_MissingFile 测试缺少文件
func TestMaterialController_UploadMaterial_MissingFile(t *testing.T) {
	setupMaterialTestDB(t)
	license := createTestLicense(t, db.GetDB())

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.POST("/materials/upload", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.UploadMaterial(c)
	})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	err := writer.WriteField("category_id", license.ID)
	if err != nil {
		t.Fatalf("Failed to write field: %v", err)
	}
	writer.Close()

	req, _ := http.NewRequest("POST", "/materials/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestMaterialController_UploadMaterial_FileTooLarge 测试文件过大
func TestMaterialController_UploadMaterial_FileTooLarge(t *testing.T) {
	setupMaterialTestDB(t)
	license := createTestLicense(t, db.GetDB())

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.POST("/materials/upload", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.UploadMaterial(c)
	})

	// 这个测试在实际中很难模拟，因为我们需要创建一个超过 10MB 的文件
	// 这里只做基本验证
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	fileWriter, _ := writer.CreateFormFile("file", "test.jpg")
	fileWriter.Write([]byte("test content"))
	err := writer.WriteField("category_id", license.ID)
	if err != nil {
		t.Fatalf("Failed to write field: %v", err)
	}
	writer.Close()

	req, _ := http.NewRequest("POST", "/materials/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 小文件应该通过大小验证
	if w.Code == http.StatusBadRequest {
		t.Errorf("Small file should pass size validation, got %d", w.Code)
	}
}

// TestMaterialController_DeleteMaterial_Success 测试删除素材成功
func TestMaterialController_DeleteMaterial_Success(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)
	material := createTestMaterial(t, database, license.ID, "")

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.DELETE("/materials/:id", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.DeleteMaterial(c)
	})

	req, _ := http.NewRequest("DELETE", "/materials/"+material.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestMaterialController_DeleteMaterial_EmptyID 测试素材 ID 为空
func TestMaterialController_DeleteMaterial_EmptyID(t *testing.T) {
	setupMaterialTestDB(t)
	license := createTestLicense(t, db.GetDB())

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.DELETE("/materials/:id", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.DeleteMaterial(c)
	})

	// Gin 路由 :id 参数不能为空，访问 /materials/ 会返回 404
	// 这里测试访问不带 ID 的路径
	req, _ := http.NewRequest("DELETE", "/materials/not-empty-but-invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 应该返回 404 因为找不到该 ID 的资源
	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status Not Found or Internal Server Error, got %d", w.Code)
	}
}

// TestMaterialController_DeleteMaterial_NotFound 测试素材不存在
func TestMaterialController_DeleteMaterial_NotFound(t *testing.T) {
	setupMaterialTestDB(t)
	license := createTestLicense(t, db.GetDB())

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.DELETE("/materials/:id", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.DeleteMaterial(c)
	})

	req, _ := http.NewRequest("DELETE", "/materials/non-existent-id", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 删除不存在的素材，返回 404 或 500
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status OK, Not Found or Internal Server Error, got %d", w.Code)
	}
}

// TestMaterialController_GetMaterialCategories_Success 测试获取素材分类列表成功
func TestMaterialController_GetMaterialCategories_Success(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)
	createTestCategory(t, database, license.ID)

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.GET("/materials/categories", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.GetMaterialCategories(c)
	})

	req, _ := http.NewRequest("GET", "/materials/categories?page=1&limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestMaterialController_GetMaterialCategories_EmptyLicense 测试许可证 ID 为空
func TestMaterialController_GetMaterialCategories_EmptyLicense(t *testing.T) {
	setupMaterialTestDB(t)
	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()
	router.GET("/materials/categories", ctrl.GetMaterialCategories)

	req, _ := http.NewRequest("GET", "/materials/categories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK (returns empty array), got %d", w.Code)
	}
}

// TestMaterialController_GetMaterialCategories_WithFilters 测试带筛选条件的分类列表
func TestMaterialController_GetMaterialCategories_WithFilters(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)
	createTestCategory(t, database, license.ID)

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.GET("/materials/categories", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.GetMaterialCategories(c)
	})

	// 测试 parent_id 筛选
	req, _ := http.NewRequest("GET", "/materials/categories?parent_id=0&type=image", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestMaterialController_CreateMaterialCategory_Success 测试创建素材分类成功
func TestMaterialController_CreateMaterialCategory_Success(t *testing.T) {
	setupMaterialTestDB(t)
	license := createTestLicense(t, db.GetDB())

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.POST("/materials/categories", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.CreateMaterialCategory(c)
	})

	createReq := dto.CreateMaterialCategoryRequest{
		Name:        "新分类",
		Type:        "image",
		ParentID:    "",
		Icon:        "icon.png",
		Color:       "#FF0000",
		Sort:        1,
		Description: "测试分类",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/materials/categories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestMaterialController_CreateMaterialCategory_Unauthorized 测试未授权访问
func TestMaterialController_CreateMaterialCategory_Unauthorized(t *testing.T) {
	setupMaterialTestDB(t)
	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()
	router.POST("/materials/categories", ctrl.CreateMaterialCategory)

	createReq := dto.CreateMaterialCategoryRequest{
		Name: "新分类",
		Type: "image",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/materials/categories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status Unauthorized, got %d", w.Code)
	}
}

// TestMaterialController_CreateMaterialCategory_InvalidJSON 测试无效 JSON
func TestMaterialController_CreateMaterialCategory_InvalidJSON(t *testing.T) {
	setupMaterialTestDB(t)
	license := createTestLicense(t, db.GetDB())

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.POST("/materials/categories", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.CreateMaterialCategory(c)
	})

	req, _ := http.NewRequest("POST", "/materials/categories", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestMaterialController_CreateMaterialCategory_InvalidType 测试无效类型
func TestMaterialController_CreateMaterialCategory_InvalidType(t *testing.T) {
	setupMaterialTestDB(t)
	license := createTestLicense(t, db.GetDB())

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.POST("/materials/categories", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.CreateMaterialCategory(c)
	})

	createReq := dto.CreateMaterialCategoryRequest{
		Name: "新分类",
		Type: "invalid_type",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/materials/categories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestMaterialController_UpdateMaterialCategory_Success 测试更新素材分类成功
func TestMaterialController_UpdateMaterialCategory_Success(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)
	category := createTestCategory(t, database, license.ID)

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.PUT("/materials/categories/:id", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.UpdateMaterialCategory(c)
	})

	updateReq := dto.UpdateMaterialCategoryRequest{
		Name:        "更新后的分类",
		Description: "更新后的描述",
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/materials/categories/"+category.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestMaterialController_UpdateMaterialCategory_EmptyID 测试分类 ID 为空
func TestMaterialController_UpdateMaterialCategory_EmptyID(t *testing.T) {
	setupMaterialTestDB(t)
	license := createTestLicense(t, db.GetDB())

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.PUT("/materials/categories/:id", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.UpdateMaterialCategory(c)
	})

	// Gin 路由 :id 参数不能为空，使用无效 ID 测试
	updateReq := dto.UpdateMaterialCategoryRequest{
		Name: "更新后的分类",
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/materials/categories/invalid-id", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 应该返回 404 因为找不到该 ID 的资源
	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status Not Found or Internal Server Error, got %d", w.Code)
	}
}

// TestMaterialController_UpdateMaterialCategory_InvalidJSON 测试无效 JSON
func TestMaterialController_UpdateMaterialCategory_InvalidJSON(t *testing.T) {
	setupMaterialTestDB(t)
	license := createTestLicense(t, db.GetDB())

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.PUT("/materials/categories/:id", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.UpdateMaterialCategory(c)
	})

	req, _ := http.NewRequest("PUT", "/materials/categories/test-id", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestMaterialController_UpdateMaterialCategory_NotFound 测试分类不存在
func TestMaterialController_UpdateMaterialCategory_NotFound(t *testing.T) {
	setupMaterialTestDB(t)
	license := createTestLicense(t, db.GetDB())

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.PUT("/materials/categories/:id", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.UpdateMaterialCategory(c)
	})

	updateReq := dto.UpdateMaterialCategoryRequest{
		Name: "更新后的分类",
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/materials/categories/non-existent-id", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status Not Found or Internal Server Error, got %d", w.Code)
	}
}

// TestMaterialController_DeleteMaterialCategory_Success 测试删除素材分类成功
func TestMaterialController_DeleteMaterialCategory_Success(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)
	category := createTestCategory(t, database, license.ID)

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.DELETE("/materials/categories/:id", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.DeleteMaterialCategory(c)
	})

	req, _ := http.NewRequest("DELETE", "/materials/categories/"+category.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestMaterialController_DeleteMaterialCategory_EmptyID 测试分类 ID 为空
func TestMaterialController_DeleteMaterialCategory_EmptyID(t *testing.T) {
	setupMaterialTestDB(t)
	license := createTestLicense(t, db.GetDB())

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.DELETE("/materials/categories/:id", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.DeleteMaterialCategory(c)
	})

	// Gin 路由 :id 参数不能为空，使用无效 ID 测试
	req, _ := http.NewRequest("DELETE", "/materials/categories/not-empty-but-invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 应该返回 404 因为找不到该 ID 的资源
	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status Not Found or Internal Server Error, got %d", w.Code)
	}
}

// TestMaterialController_DeleteMaterialCategory_NotFound 测试分类不存在
func TestMaterialController_DeleteMaterialCategory_NotFound(t *testing.T) {
	setupMaterialTestDB(t)
	license := createTestLicense(t, db.GetDB())

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.DELETE("/materials/categories/:id", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.DeleteMaterialCategory(c)
	})

	req, _ := http.NewRequest("DELETE", "/materials/categories/non-existent-id", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status Not Found or Internal Server Error, got %d", w.Code)
	}
}

// TestMaterialController_GetMaterialSelector_Success 测试获取素材选择器数据成功
func TestMaterialController_GetMaterialSelector_Success(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)
	category := createTestCategory(t, database, license.ID)
	createTestMaterial(t, database, license.ID, category.ID)

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.GET("/materials/selector", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.GetMaterialSelector(c)
	})

	req, _ := http.NewRequest("GET", "/materials/selector", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestMaterialController_GetMaterialSelector_Unauthorized 测试未授权访问
func TestMaterialController_GetMaterialSelector_Unauthorized(t *testing.T) {
	setupMaterialTestDB(t)
	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()
	router.GET("/materials/selector", ctrl.GetMaterialSelector)

	req, _ := http.NewRequest("GET", "/materials/selector", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status Unauthorized, got %d", w.Code)
	}
}

// TestMaterialController_GetMaterialSelector_WithType 测试按类型获取素材选择器
func TestMaterialController_GetMaterialSelector_WithType(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)
	category := createTestCategory(t, database, license.ID)
	createTestMaterial(t, database, license.ID, category.ID)

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.GET("/materials/selector", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.GetMaterialSelector(c)
	})

	req, _ := http.NewRequest("GET", "/materials/selector?type=image", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestMaterialController_GetMaterialSelector_EmptyData 测试空数据
func TestMaterialController_GetMaterialSelector_EmptyData(t *testing.T) {
	setupMaterialTestDB(t)
	license := createTestLicense(t, db.GetDB())

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.GET("/materials/selector", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.GetMaterialSelector(c)
	})

	req, _ := http.NewRequest("GET", "/materials/selector", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestMaterialController_MultipleMaterials 测试多个素材
func TestMaterialController_MultipleMaterials(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)
	category := createTestCategory(t, database, license.ID)

	// 创建多个不同类型的素材
	materials := []*model.Material{
		{
			Name:       "图片素材 1",
			Type:       model.MaterialTypeImage,
			CategoryID: category.ID,
			URL:        "https://example.com/image1.jpg",
			Size:       102400,
			MimeType:   "image/jpeg",
			Hash:       "hash-image-1",
			LicenseID:  license.ID,
			UserID:     "test-user",
			Status:     "active",
		},
		{
			Name:       "图片素材 2",
			Type:       model.MaterialTypeImage,
			CategoryID: category.ID,
			URL:        "https://example.com/image2.png",
			Size:       204800,
			MimeType:   "image/png",
			Hash:       "hash-image-2",
			LicenseID:  license.ID,
			UserID:     "test-user",
			Status:     "active",
		},
		{
			Name:       "视频素材 1",
			Type:       model.MaterialTypeVideo,
			CategoryID: category.ID,
			URL:        "https://example.com/video1.mp4",
			Size:       10485760,
			MimeType:   "video/mp4",
			Hash:       "hash-video-1",
			Duration:   60,
			LicenseID:  license.ID,
			UserID:     "test-user",
			Status:     "active",
		},
		{
			Name:       "音频素材 1",
			Type:       model.MaterialTypeAudio,
			CategoryID: category.ID,
			URL:        "https://example.com/audio1.mp3",
			Size:       3145728,
			MimeType:   "audio/mpeg",
			Hash:       "hash-audio-1",
			Duration:   180,
			LicenseID:  license.ID,
			UserID:     "test-user",
			Status:     "active",
		},
	}

	for _, m := range materials {
		database.Create(m)
	}

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.GET("/materials", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.GetMaterialList(c)
	})

	req, _ := http.NewRequest("GET", "/materials?page=1&limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)

	data, ok := response["data"].(map[string]any)
	if !ok {
		t.Fatal("Expected data in response")
	}

	list, ok := data["list"].([]any)
	if !ok {
		t.Fatal("Expected list in data")
	}

	if len(list) != 4 {
		t.Errorf("Expected 4 materials, got %d", len(list))
	}
}

// TestMaterialController_CategoryHierarchy 测试分类层级
func TestMaterialController_CategoryHierarchy(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)

	// 创建父子分类
	parentCategory := &model.MaterialCategory{
		Name:          "父分类",
		Type:          model.MaterialTypeImage,
		LicenseID:     license.ID,
		UserID:        "test-user",
		Status:        "active",
		MaterialCount: 0,
	}
	database.Create(parentCategory)

	childCategory := &model.MaterialCategory{
		Name:          "子分类",
		Type:          model.MaterialTypeImage,
		ParentID:      parentCategory.ID,
		LicenseID:     license.ID,
		UserID:        "test-user",
		Status:        "active",
		MaterialCount: 0,
	}
	database.Create(childCategory)

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.GET("/materials/categories", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.GetMaterialCategories(c)
	})

	// 获取所有分类
	req, _ := http.NewRequest("GET", "/materials/categories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}

	// 获取子分类
	req, _ = http.NewRequest("GET", "/materials/categories?parent_id="+parentCategory.ID, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestMaterialController_EdgeCases 测试边界条件
func TestMaterialController_EdgeCases(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	// 测试分页边界
	router.GET("/materials", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.GetMaterialList(c)
	})

	// 测试 page=0
	req, _ := http.NewRequest("GET", "/materials?page=0&limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK for page=0, got %d", w.Code)
	}

	// 测试 limit=0
	req, _ = http.NewRequest("GET", "/materials?page=1&limit=0", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK for limit=0, got %d", w.Code)
	}

	// 测试负数分页
	req, _ = http.NewRequest("GET", "/materials?page=-1&limit=-10", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK for negative values, got %d", w.Code)
	}
}

// TestMaterialController_ConcurrentAccess 测试并发访问
func TestMaterialController_ConcurrentAccess(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)
	category := createTestCategory(t, database, license.ID)
	createTestMaterial(t, database, license.ID, category.ID)

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.GET("/materials", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.GetMaterialList(c)
	})

	// 并发发送多个请求，使用 wait group 等待完成
	var wg sync.WaitGroup
	errors := make(chan error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("GET", "/materials", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				errors <- fmt.Errorf("Expected status OK, got %d", w.Code)
				return
			}
		}()
	}

	// 等待所有请求完成
	wg.Wait()
	close(errors)

	// 报告任何错误
	for err := range errors {
		t.Error(err)
	}
}

// TestMaterialController_SpecialCharacters 测试特殊字符处理
func TestMaterialController_SpecialCharacters(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)
	category := createTestCategory(t, database, license.ID)

	// 创建包含特殊字符的素材
	material := &model.Material{
		Name:        "测试素材<script>alert('xss')</script>",
		Type:        model.MaterialTypeImage,
		CategoryID:  category.ID,
		URL:         "https://example.com/test.jpg",
		Size:        102400,
		MimeType:    "image/jpeg",
		Hash:        "test-hash",
		LicenseID:   license.ID,
		UserID:      "test-user",
		Status:      "active",
		Description: "包含特殊字符 & < > \" ' 的测试",
	}
	database.Create(material)

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.GET("/materials", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.GetMaterialList(c)
	})

	req, _ := http.NewRequest("GET", "/materials?search=<script>", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestMaterialController_LongStrings 测试长字符串处理
func TestMaterialController_LongStrings(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.POST("/materials/categories", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.CreateMaterialCategory(c)
	})

	// 创建长名称分类
	longName := strings.Repeat("a", 500)
	createReq := dto.CreateMaterialCategoryRequest{
		Name:        longName,
		Type:        "image",
		Description: strings.Repeat("b", 1000),
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/materials/categories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 应该能处理长字符串或返回验证错误
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Errorf("Expected status OK or Bad Request, got %d", w.Code)
	}
}

// TestMaterialController_CategoryTypes 测试不同分类类型
func TestMaterialController_CategoryTypes(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)

	ctrl := newMaterialControllerForTest()

	// 测试所有有效的分类类型
	validTypes := []string{"image", "video", "audio", "file"}

	for _, matType := range validTypes {
		category := &model.MaterialCategory{
			Name:          matType + "分类",
			Type:          model.MaterialType(matType),
			LicenseID:     license.ID,
			UserID:        "test-user",
			Status:        "active",
			MaterialCount: 0,
		}
		database.Create(category)
	}

	router := setupGinEngine()

	router.GET("/materials/categories", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.GetMaterialCategories(c)
	})

	// 测试按类型筛选
	for _, matType := range validTypes {
		req, _ := http.NewRequest("GET", "/materials/categories?type="+matType, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status OK for type %s, got %d", matType, w.Code)
		}
	}
}

// TestMaterialController_MaterialUsageTracking 测试素材使用追踪
func TestMaterialController_MaterialUsageTracking(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)
	material := createTestMaterial(t, database, license.ID, "")

	// 初始使用次数应为 0
	if material.UsageCount != 0 {
		t.Errorf("Expected initial usage count to be 0, got %d", material.UsageCount)
	}

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.PUT("/materials/:id/usage", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.UpdateMaterialUsage(c)
	})

	// 多次更新使用次数
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest("PUT", "/materials/"+material.ID+"/usage", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %d", w.Code)
		}
	}

	// 验证使用次数已更新
	var updatedMaterial model.Material
	database.Where("id = ?", material.ID).First(&updatedMaterial)

	if updatedMaterial.UsageCount != 3 {
		t.Errorf("Expected usage count to be 3, got %d", updatedMaterial.UsageCount)
	}

	if updatedMaterial.LastUsedAt == nil {
		t.Error("Expected LastUsedAt to be set")
	}
}

// TestMaterialController_StatsCalculation 统计数据计算
func TestMaterialController_StatsCalculation(t *testing.T) {
	database := setupMaterialTestDB(t)
	license := createTestLicense(t, database)

	// 创建不同类型的素材
	materials := []*model.Material{
		{
			Name:       "图片 1",
			Type:       model.MaterialTypeImage,
			URL:        "https://example.com/1.jpg",
			Size:       1000,
			MimeType:   "image/jpeg",
			Hash:       "hash1",
			LicenseID:  license.ID,
			UserID:     "test-user",
			UsageCount: 5,
			Status:     "active",
		},
		{
			Name:       "图片 2",
			Type:       model.MaterialTypeImage,
			URL:        "https://example.com/2.png",
			Size:       2000,
			MimeType:   "image/png",
			Hash:       "hash2",
			LicenseID:  license.ID,
			UserID:     "test-user",
			UsageCount: 3,
			Status:     "active",
		},
		{
			Name:       "视频 1",
			Type:       model.MaterialTypeVideo,
			URL:        "https://example.com/1.mp4",
			Size:       10000,
			MimeType:   "video/mp4",
			Hash:       "hash3",
			LicenseID:  license.ID,
			UserID:     "test-user",
			UsageCount: 2,
			Status:     "active",
		},
	}

	for _, m := range materials {
		database.Create(m)
	}

	ctrl := newMaterialControllerForTest()
	router := setupGinEngine()

	router.GET("/materials/stats", func(c *gin.Context) {
		c.Set("license_id", license.ID)
		c.Set("user_id", "test-user")
		ctrl.GetMaterialStats(c)
	})

	req, _ := http.NewRequest("GET", "/materials/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)

	data, ok := response["data"].(map[string]any)
	if !ok {
		t.Fatal("Expected data in response")
	}

	// 验证统计信息
	totalMaterials, ok := data["total_materials"].(float64)
	if !ok || int(totalMaterials) != 3 {
		t.Errorf("Expected total_materials to be 3, got %v", data["total_materials"])
	}

	imageCount, ok := data["image_count"].(float64)
	if !ok || int(imageCount) != 2 {
		t.Errorf("Expected image_count to be 2, got %v", data["image_count"])
	}

	videoCount, ok := data["video_count"].(float64)
	if !ok || int(videoCount) != 1 {
		t.Errorf("Expected video_count to be 1, got %v", data["video_count"])
	}

	totalSize, ok := data["total_size"].(float64)
	if !ok || int(totalSize) != 13000 {
		t.Errorf("Expected total_size to be 13000, got %v", data["total_size"])
	}
}
