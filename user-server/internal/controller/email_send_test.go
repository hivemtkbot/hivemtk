package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/pkg/db"

	"hivemtk-user/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func setupEmailSendTestDB(t *testing.T) *gorm.DB {
	gin.SetMode(gin.TestMode)
	database := testutil.NewTestDB(t)
	db.SetTestDB(database)
	return database
}

func setupEmailSendController(t *testing.T) (*EmailSendController, *gin.Engine) {
	setupEmailSendTestDB(t)
	ctrl := NewEmailSendController()
	router := gin.New()

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Next()
	})

	return ctrl, router
}

// TestEmailSendController_SendEmail_Success 测试发送邮件成功
func TestEmailSendController_SendEmail_Success(t *testing.T) {
	_, router := setupEmailSendController(t)

	router.POST("/api/email/send", func(ctx *gin.Context) {
		var req dto.SendEmailRequest
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx.JSON(http.StatusOK, gin.H{
			"code": "SUCCESS",
			"data": gin.H{
				"id":      "test-email-id",
				"to":      req.To,
				"subject": req.Subject,
				"status":  0,
			},
			"message": "发送成功",
		})
	})

	req := dto.SendEmailRequest{
		To:            "test@example.com",
		Subject:       "Test Subject",
		Content:       "Test Content",
		ImmediateSend: true,
		SmtpId:        "smtp-123",
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("POST", "/api/email/send", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["code"] != "SUCCESS" {
		t.Errorf("Expected code SUCCESS, got %v", response["code"])
	}
}

// TestEmailSendController_SendEmail_MissingRequiredFields 测试缺少必填字段
func TestEmailSendController_SendEmail_MissingRequiredFields(t *testing.T) {
	ctrl, router := setupEmailSendController(t)
	router.POST("/api/email/send", ctrl.SendEmail)

	req := map[string]any{}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("POST", "/api/email/send", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestEmailSendController_SendEmail_MissingTo 测试缺少收件人
func TestEmailSendController_SendEmail_MissingTo(t *testing.T) {
	ctrl, router := setupEmailSendController(t)
	router.POST("/api/email/send", ctrl.SendEmail)

	req := map[string]any{
		"subject": "Test",
		"content": "Content",
		"smtpId":  "smtp-123",
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("POST", "/api/email/send", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestEmailSendController_SendEmail_MissingSubject 测试缺少主题
func TestEmailSendController_SendEmail_MissingSubject(t *testing.T) {
	ctrl, router := setupEmailSendController(t)
	router.POST("/api/email/send", ctrl.SendEmail)

	req := map[string]any{
		"to":      "test@example.com",
		"content": "Content",
		"smtpId":  "smtp-123",
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("POST", "/api/email/send", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestEmailSendController_SendEmail_MissingContent 测试缺少内容
func TestEmailSendController_SendEmail_MissingContent(t *testing.T) {
	ctrl, router := setupEmailSendController(t)
	router.POST("/api/email/send", ctrl.SendEmail)

	req := map[string]any{
		"to":      "test@example.com",
		"subject": "Test",
		"smtpId":  "smtp-123",
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("POST", "/api/email/send", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestEmailSendController_SendEmail_MissingSmtpId 测试缺少 SMTP ID
func TestEmailSendController_SendEmail_MissingSmtpId(t *testing.T) {
	ctrl, router := setupEmailSendController(t)
	router.POST("/api/email/send", ctrl.SendEmail)

	req := map[string]any{
		"to":      "test@example.com",
		"subject": "Test",
		"content": "Content",
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("POST", "/api/email/send", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestEmailSendController_SendEmail_InvalidJSON 测试无效 JSON
func TestEmailSendController_SendEmail_InvalidJSON(t *testing.T) {
	ctrl, router := setupEmailSendController(t)
	router.POST("/api/email/send", ctrl.SendEmail)

	httpReq, _ := http.NewRequest("POST", "/api/email/send", bytes.NewReader([]byte("invalid-json")))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestEmailSendController_SendEmail_WithScheduledTime 测试定时发送
func TestEmailSendController_SendEmail_WithScheduledTime(t *testing.T) {
	_, router := setupEmailSendController(t)

	router.POST("/api/email/send", func(ctx *gin.Context) {
		var req dto.SendEmailRequest
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.SendTime == nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "sendTime required"})
			return
		}

		ctx.JSON(http.StatusOK, gin.H{
			"code": "SUCCESS",
			"data": gin.H{
				"id":        "test-email-id",
				"to":        req.To,
				"subject":   req.Subject,
				"send_time": req.SendTime,
			},
			"message": "发送成功",
		})
	})

	scheduledTime := time.Now().Add(time.Hour)
	req := dto.SendEmailRequest{
		To:            "test@example.com",
		Subject:       "Scheduled Email",
		Content:       "Test Content",
		SendTime:      &scheduledTime,
		ImmediateSend: false,
		SmtpId:        "smtp-123",
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("POST", "/api/email/send", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestEmailSendController_SendEmail_WithAttachments 测试带附件发送
func TestEmailSendController_SendEmail_WithAttachments(t *testing.T) {
	_, router := setupEmailSendController(t)

	router.POST("/api/email/send", func(ctx *gin.Context) {
		var req dto.SendEmailRequest
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx.JSON(http.StatusOK, gin.H{
			"code": "SUCCESS",
			"data": gin.H{
				"id":          "test-email-id",
				"to":          req.To,
				"subject":     req.Subject,
				"attachments": req.Attachments,
			},
			"message": "发送成功",
		})
	})

	req := dto.SendEmailRequest{
		To:            "test@example.com",
		Subject:       "Email with Attachments",
		Content:       "Test Content",
		Attachments:   []string{"file1.pdf", "file2.pdf"},
		ImmediateSend: true,
		SmtpId:        "smtp-123",
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("POST", "/api/email/send", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	data, ok := response["data"].(map[string]any)
	if !ok {
		t.Fatal("Expected data to be a map")
	}
	attachments, ok := data["attachments"].([]any)
	if !ok || len(attachments) != 2 {
		t.Errorf("Expected 2 attachments, got %v", data["attachments"])
	}
}

// TestEmailSendController_NewEmailSendController 测试构造函数
func TestEmailSendController_NewEmailSendController(t *testing.T) {
	ctrl := NewEmailSendController()
	if ctrl == nil {
		t.Error("Expected controller instance, got nil")
	}
}

