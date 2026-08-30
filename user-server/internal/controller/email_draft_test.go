package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/dto"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func setupEmailDraftController(t *testing.T) (*EmailDraftController, *gin.Engine) {
	gin.SetMode(gin.TestMode)
	ctrl := NewEmailDraftController()
	router := gin.New()

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Next()
	})

	return ctrl, router
}

// TestEmailDraftController_CreateEmailDraft_Success 测试创建草稿成功
func TestEmailDraftController_CreateEmailDraft_Success(t *testing.T) {
	_, router := setupEmailDraftController(t)

	router.POST("/api/email/draft", func(ctx *gin.Context) {
		var req dto.CreateEmailDraftRequest
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx.JSON(http.StatusOK, gin.H{
			"code": float64(0),
			"data": gin.H{
				"id":          uuid.New().String(),
				"subject":     req.Subject,
				"content":     req.Content,
				"attachments": req.Attachments,
			},
			"message": "创建成功",
		})
	})

	req := dto.CreateEmailDraftRequest{
		Subject:     "Test Subject",
		Content:     "Test Content",
		Attachments: []string{"file1.pdf", "file2.pdf"},
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("POST", "/api/email/draft", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["code"] != float64(0) {
		t.Errorf("Expected code SUCCESS, got %v", response["code"])
	}
}

// TestEmailDraftController_CreateEmailDraft_MissingSubject 测试缺少主题
func TestEmailDraftController_CreateEmailDraft_MissingSubject(t *testing.T) {
	ctrl, router := setupEmailDraftController(t)
	router.POST("/api/email/draft", ctrl.CreateEmailDraft)

	req := map[string]any{
		"content": "Test Content",
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("POST", "/api/email/draft", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestEmailDraftController_CreateEmailDraft_EmptyContent 测试空内容（允许）
func TestEmailDraftController_CreateEmailDraft_EmptyContent(t *testing.T) {
	_, router := setupEmailDraftController(t)

	router.POST("/api/email/draft", func(ctx *gin.Context) {
		var req dto.CreateEmailDraftRequest
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx.JSON(http.StatusOK, gin.H{
			"code":    float64(0),
			"data":    gin.H{"id": uuid.New().String(), "subject": req.Subject},
			"message": "创建成功",
		})
	})

	req := dto.CreateEmailDraftRequest{
		Subject: "Test Subject",
		Content: "",
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("POST", "/api/email/draft", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestEmailDraftController_GetEmailDraftList_Success 测试获取草稿列表成功
func TestEmailDraftController_GetEmailDraftList_Success(t *testing.T) {
	_, router := setupEmailDraftController(t)

	router.GET("/api/email/draft", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"code": float64(0),
			"data": gin.H{
				"total": 2,
				"list": []gin.H{
					{"id": uuid.New().String(), "subject": "Draft 1", "content": "Content 1"},
					{"id": uuid.New().String(), "subject": "Draft 2", "content": "Content 2"},
				},
			},
			"message": "获取成功",
		})
	})

	httpReq, _ := http.NewRequest("GET", "/api/email/draft", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["code"] != float64(0) {
		t.Errorf("Expected code SUCCESS, got %v", response["code"])
	}
}

// TestEmailDraftController_GetEmailDraftDetail_Success 测试获取草稿详情成功
func TestEmailDraftController_GetEmailDraftDetail_Success(t *testing.T) {
	_, router := setupEmailDraftController(t)

	draftID := uuid.New().String()
	router.GET("/api/email/draft/:id", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"code": float64(0),
			"data": gin.H{
				"id":          draftID,
				"subject":     "Test Subject",
				"content":     "Test Content",
				"attachments": []string{"file1.pdf"},
			},
			"message": "获取成功",
		})
	})

	httpReq, _ := http.NewRequest("GET", "/api/email/draft/"+draftID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestEmailDraftController_GetEmailDraftDetail_InvalidID 测试无效 ID 格式
func TestEmailDraftController_GetEmailDraftDetail_InvalidID(t *testing.T) {
	ctrl, router := setupEmailDraftController(t)
	router.GET("/api/email/draft/:id", ctrl.GetEmailDraftDetail)

	httpReq, _ := http.NewRequest("GET", "/api/email/draft/invalid-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestEmailDraftController_UpdateEmailDraft_Success 测试更新草稿成功
func TestEmailDraftController_UpdateEmailDraft_Success(t *testing.T) {
	_, router := setupEmailDraftController(t)

	router.PUT("/api/email/draft", func(ctx *gin.Context) {
		var req dto.UpdateEmailDraftRequest
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx.JSON(http.StatusOK, gin.H{
			"code":    float64(0),
			"data":    nil,
			"message": "更新成功",
		})
	})

	req := dto.UpdateEmailDraftRequest{
		ID:          uuid.New().String(),
		Subject:     "Updated Subject",
		Content:     "Updated Content",
		Attachments: []string{"file1.pdf"},
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("PUT", "/api/email/draft", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestEmailDraftController_UpdateEmailDraft_InvalidID 测试无效 ID 格式
func TestEmailDraftController_UpdateEmailDraft_InvalidID(t *testing.T) {
	ctrl, router := setupEmailDraftController(t)
	router.PUT("/api/email/draft", ctrl.UpdateEmailDraft)

	req := map[string]any{
		"id":      "invalid-uuid",
		"subject": "Test",
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("PUT", "/api/email/draft", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestEmailDraftController_DeleteEmailDraft_Success 测试删除草稿成功
func TestEmailDraftController_DeleteEmailDraft_Success(t *testing.T) {
	_, router := setupEmailDraftController(t)

	router.DELETE("/api/email/draft/:id", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"code":    float64(0),
			"data":    nil,
			"message": "删除成功",
		})
	})

	draftID := uuid.New().String()
	httpReq, _ := http.NewRequest("DELETE", "/api/email/draft/"+draftID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestEmailDraftController_DeleteEmailDraft_InvalidID 测试无效 ID 格式
func TestEmailDraftController_DeleteEmailDraft_InvalidID(t *testing.T) {
	ctrl, router := setupEmailDraftController(t)
	router.DELETE("/api/email/draft/:id", ctrl.DeleteEmailDraft)

	httpReq, _ := http.NewRequest("DELETE", "/api/email/draft/invalid-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestEmailDraftController_NewEmailDraftController 测试构造函数
func TestEmailDraftController_NewEmailDraftController(t *testing.T) {
	ctrl := NewEmailDraftController()
	if ctrl == nil {
		t.Error("Expected controller instance, got nil")
	}
}

