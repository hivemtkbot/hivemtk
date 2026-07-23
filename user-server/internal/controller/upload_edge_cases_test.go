package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUploadFile_EmptyMultipartForm(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/upload", UploadFile)

	body := &bytes.Buffer{}
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", "multipart/form-data")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for empty multipart, got %d", w.Code)
	}
}

func TestUploadFile_NoContentType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/upload", UploadFile)

	body := &bytes.Buffer{}
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", "text/plain")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for wrong content type, got %d", w.Code)
	}
}

func TestUploadFile_MultipleDangerousExtensions(t *testing.T) {
	setupUploadTestDir(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/upload", UploadFile)

	dangerousExts := []string{".exe", ".bat", ".cmd", ".sh", ".php", ".jsp", ".asp", ".py", ".rb"}
	for _, ext := range dangerousExts {
		req := createUploadMultipartRequest(t, "file", "test"+ext, []byte{0x00})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected 400 for %s, got %d", ext, w.Code)
		}
	}
}

func TestUploadFile_MultipleValidExtensions(t *testing.T) {
	setupUploadTestDir(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/upload", UploadFile)

	validCases := []struct {
		ext     string
		content []byte
	}{
		{".jpg", []byte{0xFF, 0xD8, 0xFF, 0xE0}},
		{".png", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}},
		{".gif", []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61}},
		{".pdf", []byte{0x25, 0x50, 0x44, 0x46, 0x2D}},
		{".webp", []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50}},
	}

	for _, tc := range validCases {
		req := createUploadMultipartRequest(t, "file", "test"+tc.ext, tc.content)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200 for %s, got %d. Body: %s", tc.ext, w.Code, w.Body.String())
		}
	}
}
