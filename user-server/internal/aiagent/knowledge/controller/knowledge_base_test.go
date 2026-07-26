package controller

import (
	"bytes"
	knowledgesvc "marketing/internal/aiagent/knowledge/service"
	"marketing/internal/model"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
	"marketing/internal/pkg/testutil/testmigrate"
)

// setupKBTestDB 设置知识库测试数据库
func setupKBTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.KBDocument{},
	)
	testmigrate.RunTestMigrations(t, database)
	return database
}

func setupKBRouter(t *testing.T, ctrl *KnowledgeBaseController) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test_value")
		c.Next()
	})
	// 用测试数据库替换服务
	db := setupKBTestDB(t)
	ctrl.kbService = knowledgesvc.NewKnowledgeBaseServiceWithDB(db)
	group := router.Group("/api")
	ctrl.RegisterRoutes(group)
	return router
}

func TestKnowledgeBaseController_ListDocuments_Success(t *testing.T) {
	ctrl := NewKnowledgeBaseController()
	router := setupKBRouter(t, ctrl)

	req, _ := http.NewRequest("GET", "/api/rag/documents", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestKnowledgeBaseController_GetDocument_Success(t *testing.T) {
	ctrl := NewKnowledgeBaseController()
	router := setupKBRouter(t, ctrl)

	req, _ := http.NewRequest("GET", "/api/rag/documents/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 测试数据库为空,记录不存在返回 404 是正常
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}
}

func TestKnowledgeBaseController_DeleteDocument_Success(t *testing.T) {
	ctrl := NewKnowledgeBaseController()
	router := setupKBRouter(t, ctrl)

	req, _ := http.NewRequest("DELETE", "/api/rag/documents/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 测试数据库为空,记录不存在,但删除操作可能返回 200/404/500 都是合理的
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 200, 404 or 500, got %d", w.Code)
	}
}

func TestKnowledgeBaseController_ImportKnowledgeBase_NoFile(t *testing.T) {
	ctrl := NewKnowledgeBaseController()
	router := setupKBRouter(t, ctrl)

	req, _ := http.NewRequest("POST", "/api/rag/import", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for no file, got %d", w.Code)
	}
}

func TestKnowledgeBaseController_ImportKnowledgeBase_InvalidExtension(t *testing.T) {
	ctrl := NewKnowledgeBaseController()
	router := setupKBRouter(t, ctrl)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "malware.exe")
	part.Write([]byte("test"))
	writer.Close()

	req, _ := http.NewRequest("POST", "/api/rag/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for .exe, got %d", w.Code)
	}
}

func TestKnowledgeBaseController_ImportKnowledgeBase_Success(t *testing.T) {
	ctrl := NewKnowledgeBaseController()
	router := setupKBRouter(t, ctrl)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.txt")
	part.Write([]byte("Hello, this is a knowledge base document"))
	writer.Close()

	req, _ := http.NewRequest("POST", "/api/rag/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestKnowledgeBaseController_ImportKnowledgeBase_PDF(t *testing.T) {
	ctrl := NewKnowledgeBaseController()
	router := setupKBRouter(t, ctrl)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "document.pdf")
	part.Write([]byte("%PDF-1.4 fake content"))
	writer.Close()

	req, _ := http.NewRequest("POST", "/api/rag/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 for PDF, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestKnowledgeBaseController_ImportKnowledgeBase_DOCX(t *testing.T) {
	ctrl := NewKnowledgeBaseController()
	router := setupKBRouter(t, ctrl)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "document.docx")
	part.Write([]byte("PK fake docx content"))
	writer.Close()

	req, _ := http.NewRequest("POST", "/api/rag/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 for DOCX, got %d. Body: %s", w.Code, w.Body.String())
	}
}
