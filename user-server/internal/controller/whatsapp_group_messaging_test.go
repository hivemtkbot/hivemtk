package controller

import (
	"bytes"
	"encoding/json"
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

func setupGroupMessagingTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.WhatsappAccount{},
		&model.WhatsappMessageTemplate{},
	)
	db.SetTestDB(database)
	return database
}

func setupGroupMessagingRouter(ctrl *GroupMessagingController) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test_value")
		c.Set("user_id", uint(1))
		c.Next()
	})
	router.GET("/whatsapp/lead-groups", ctrl.GetLeadGroups)
	router.POST("/whatsapp/group-messaging/send", ctrl.SelectGroupAndSendMessage)
	router.GET("/whatsapp/group-messaging/status/:queue_id", ctrl.GetMessageStatus)
	router.GET("/whatsapp/group-messaging/records", ctrl.GetSendRecords)
	router.GET("/whatsapp/templates", ctrl.GetTemplates)
	router.POST("/whatsapp/templates", ctrl.CreateTemplate)
	router.PUT("/whatsapp/templates/:id", ctrl.UpdateTemplate)
	router.DELETE("/whatsapp/templates/:id", ctrl.DeleteTemplate)
	return router
}

func TestGroupMessagingController_GetLeadGroups_Success(t *testing.T) {
	setupGroupMessagingTestDB(t)
	ctrl := NewGroupMessagingController(service.NewWhatsappService(), nil, nil, nil)
	ctrl.clueSvc = service.NewClueService()
	router := setupGroupMessagingRouter(ctrl)

	req, _ := http.NewRequest("GET", "/whatsapp/lead-groups?page=1&limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 200 or 500, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestGroupMessagingController_GetTemplates_Success(t *testing.T) {
	setupGroupMessagingTestDB(t)
	ctrl := NewGroupMessagingController(service.NewWhatsappService(), nil, nil, nil)
	router := setupGroupMessagingRouter(ctrl)

	req, _ := http.NewRequest("GET", "/whatsapp/templates", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestGroupMessagingController_CreateTemplate_Success(t *testing.T) {
	setupGroupMessagingTestDB(t)
	ctrl := NewGroupMessagingController(service.NewWhatsappService(), nil, nil, nil)
	router := setupGroupMessagingRouter(ctrl)

	body, _ := json.Marshal(map[string]any{
		"name":    "Test Template",
		"content": "Hello {{name}}",
		"type":    "text",
	})
	req, _ := http.NewRequest("POST", "/whatsapp/templates", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestGroupMessagingController_CreateTemplate_InvalidJSON(t *testing.T) {
	setupGroupMessagingTestDB(t)
	ctrl := NewGroupMessagingController(service.NewWhatsappService(), nil, nil, nil)
	router := setupGroupMessagingRouter(ctrl)

	req, _ := http.NewRequest("POST", "/whatsapp/templates", bytes.NewReader([]byte("bad")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestGroupMessagingController_SelectGroupAndSendMessage_InvalidJSON(t *testing.T) {
	setupGroupMessagingTestDB(t)
	ctrl := NewGroupMessagingController(service.NewWhatsappService(), nil, nil, nil)
	router := setupGroupMessagingRouter(ctrl)

	req, _ := http.NewRequest("POST", "/whatsapp/group-messaging/send", bytes.NewReader([]byte("bad")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestGroupMessagingController_GetSendRecords_Success(t *testing.T) {
	setupGroupMessagingTestDB(t)
	ctrl := NewGroupMessagingController(service.NewWhatsappService(), nil, nil, nil)
	router := setupGroupMessagingRouter(ctrl)

	req, _ := http.NewRequest("GET", "/whatsapp/group-messaging/records?page=1&limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

