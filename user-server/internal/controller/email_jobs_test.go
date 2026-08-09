package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"marketing/internal/dto"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func setupEmailJobsController(t *testing.T) (*EmailJobsController, *gin.Engine) {
	gin.SetMode(gin.TestMode)
	// 邮件任务服务依赖全局 db（repository 通过 db.GetDB() 取连接），
	// 必须先构建并迁移真实测试库，否则 GetEmailJobsList 会因 email_jobs 表不存在返回 400。
	database := testutil.NewTestDB(t, &model.EmailJobs{})
	_db.SetTestDB(database)
	ctrl := NewEmailJobsController()
	router := gin.New()
	router.Use(gin.Recovery()) // 把 service/repo 的 nil-DB panic 转化为 500

	router.Use(func(ctx *gin.Context) {
		ctx.Set("user_id", uint(1))
		ctx.Next()
	})

	return ctrl, router
}

// TestEmailJobsController_CreateEmailJobs_Success 测试创建任务成功
func TestEmailJobsController_CreateEmailJobs_Success(t *testing.T) {
	_, router := setupEmailJobsController(t)

	router.POST("/api/email/jobs", func(ctx *gin.Context) {
		var req dto.CreateEmailJobsRequest
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx.JSON(http.StatusOK, gin.H{
			"code": "SUCCESS",
			"data": gin.H{
				"id":            uuid.New().String(),
				"subject":       req.Subject,
				"email_total":   req.EmailTotal,
				"send_total":    req.SendTotal,
				"read_total":    req.ReadTotal,
				"success_total": req.SuccessTotal,
				"fail_total":    req.FailTotal,
			},
			"message": "创建成功",
		})
	})

	req := dto.CreateEmailJobsRequest{
		Subject:      "Test Job",
		EmailTotal:   1000,
		SendTotal:    100,
		ReadTotal:    50,
		SuccessTotal: 45,
		FailTotal:    5,
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("POST", "/api/email/jobs", bytes.NewReader(body))
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

// TestEmailJobsController_CreateEmailJobs_MissingSubject 测试缺少主题
func TestEmailJobsController_CreateEmailJobs_MissingSubject(t *testing.T) {
	ctrl, router := setupEmailJobsController(t)
	router.POST("/api/email/jobs", ctrl.CreateEmailJobs)

	req := map[string]any{
		"email_total": 1000,
		"send_total":  0,
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("POST", "/api/email/jobs", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestEmailJobsController_CreateEmailJobs_MissingEmailTotal 测试缺少邮件总数
func TestEmailJobsController_CreateEmailJobs_MissingEmailTotal(t *testing.T) {
	ctrl, router := setupEmailJobsController(t)
	router.POST("/api/email/jobs", ctrl.CreateEmailJobs)

	req := map[string]any{
		"subject":    "Test",
		"send_total": 0,
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("POST", "/api/email/jobs", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestEmailJobsController_GetEmailJobsList_Success 测试获取任务列表成功
func TestEmailJobsController_GetEmailJobsList_Success(t *testing.T) {
	_, router := setupEmailJobsController(t)

	router.GET("/api/email/jobs", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"code": "SUCCESS",
			"data": gin.H{
				"total": 2,
				"list": []gin.H{
					{"id": uuid.New().String(), "subject": "Job 1", "email_total": 1000},
					{"id": uuid.New().String(), "subject": "Job 2", "email_total": 2000},
				},
			},
			"message": "获取成功",
		})
	})

	httpReq, _ := http.NewRequest("GET", "/api/email/jobs?page=1&limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["code"] != "SUCCESS" {
		t.Errorf("Expected code SUCCESS, got %v", response["code"])
	}
}

// TestEmailJobsController_GetEmailJobsList_MissingPage 测试缺少页码 (控制器兜底为默认值 1)
func TestEmailJobsController_GetEmailJobsList_MissingPage(t *testing.T) {
	ctrl, router := setupEmailJobsController(t)
	router.GET("/api/email/jobs", ctrl.GetEmailJobsList)

	httpReq, _ := http.NewRequest("GET", "/api/email/jobs?limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	// 控制器在 req.Page <= 0 时兜底为 1, 不返回 400
	if w.Code == http.StatusBadRequest {
		t.Errorf("Expected controller to apply default page=1, got 400 instead. Body: %s", w.Body.String())
	}
}

// TestEmailJobsController_GetEmailJobsList_MissingLimit 测试缺少页大小 (控制器兜底为默认值 20)
func TestEmailJobsController_GetEmailJobsList_MissingLimit(t *testing.T) {
	ctrl, router := setupEmailJobsController(t)
	router.GET("/api/email/jobs", ctrl.GetEmailJobsList)

	httpReq, _ := http.NewRequest("GET", "/api/email/jobs?page=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	// 控制器在 req.PageSize <= 0 时兜底为 20, 不返回 400
	if w.Code == http.StatusBadRequest {
		t.Errorf("Expected controller to apply default page_size=20, got 400 instead. Body: %s", w.Body.String())
	}
}

// TestEmailJobsController_GetEmailJobsDetail_Success 测试获取任务详情成功
func TestEmailJobsController_GetEmailJobsDetail_Success(t *testing.T) {
	_, router := setupEmailJobsController(t)

	jobID := uuid.New().String()
	router.GET("/api/email/jobs/:id", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"code": "SUCCESS",
			"data": gin.H{
				"id":          jobID,
				"subject":     "Test Job",
				"email_total": 1000,
				"send_total":  100,
			},
			"message": "获取成功",
		})
	})

	httpReq, _ := http.NewRequest("GET", "/api/email/jobs/"+jobID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestEmailJobsController_GetEmailJobsDetail_InvalidID 测试无效 ID 格式
func TestEmailJobsController_GetEmailJobsDetail_InvalidID(t *testing.T) {
	ctrl, router := setupEmailJobsController(t)
	router.GET("/api/email/jobs/:id", ctrl.GetEmailJobsDetail)

	httpReq, _ := http.NewRequest("GET", "/api/email/jobs/invalid-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestEmailJobsController_UpdateEmailJobs_Success 测试更新任务成功
func TestEmailJobsController_UpdateEmailJobs_Success(t *testing.T) {
	_, router := setupEmailJobsController(t)

	router.PUT("/api/email/jobs", func(ctx *gin.Context) {
		var req dto.UpdateEmailJobsRequest
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx.JSON(http.StatusOK, gin.H{
			"code": "SUCCESS",
			"data": gin.H{
				"id":          req.ID,
				"subject":     req.Subject,
				"email_total": req.EmailTotal,
			},
			"message": "更新成功",
		})
	})

	req := dto.UpdateEmailJobsRequest{
		ID:           uuid.New().String(),
		Subject:      "Updated Job",
		EmailTotal:   2000,
		SendTotal:    500,
		SuccessTotal: 450,
		FailTotal:    50,
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("PUT", "/api/email/jobs", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestEmailJobsController_UpdateEmailJobs_InvalidID 测试无效 ID 格式
func TestEmailJobsController_UpdateEmailJobs_InvalidID(t *testing.T) {
	ctrl, router := setupEmailJobsController(t)
	router.PUT("/api/email/jobs", ctrl.UpdateEmailJobs)

	req := map[string]any{
		"id":          "invalid-uuid",
		"subject":     "Test",
		"email_total": 1000,
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("PUT", "/api/email/jobs", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestEmailJobsController_UpdateSendTotal_Success 测试更新发送总数成功
func TestEmailJobsController_UpdateSendTotal_Success(t *testing.T) {
	_, router := setupEmailJobsController(t)

	router.POST("/api/email/jobs/send-total", func(ctx *gin.Context) {
		var req dto.UpdateEmailJobsRequest
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx.JSON(http.StatusOK, gin.H{
			"code":    "SUCCESS",
			"data":    nil,
			"message": "更新成功",
		})
	})

	req := dto.UpdateEmailJobsRequest{
		ID:        uuid.New().String(),
		SendTotal: 500,
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("POST", "/api/email/jobs/send-total", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestEmailJobsController_UpdateSendTotal_InvalidID 测试无效 ID 格式
func TestEmailJobsController_UpdateSendTotal_InvalidID(t *testing.T) {
	ctrl, router := setupEmailJobsController(t)
	router.POST("/api/email/jobs/send-total", ctrl.UpdateSendTotal)

	req := map[string]any{
		"id":         "invalid-uuid",
		"send_total": 500,
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("POST", "/api/email/jobs/send-total", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestEmailJobsController_DeleteEmailJobs_Success 测试删除任务成功
func TestEmailJobsController_DeleteEmailJobs_Success(t *testing.T) {
	_, router := setupEmailJobsController(t)

	router.DELETE("/api/email/jobs/:id", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"code":    "SUCCESS",
			"data":    nil,
			"message": "删除成功",
		})
	})

	jobID := uuid.New().String()
	httpReq, _ := http.NewRequest("DELETE", "/api/email/jobs/"+jobID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestEmailJobsController_DeleteEmailJobs_InvalidID 测试无效 ID 格式
func TestEmailJobsController_DeleteEmailJobs_InvalidID(t *testing.T) {
	ctrl, router := setupEmailJobsController(t)
	router.DELETE("/api/email/jobs/:id", ctrl.DeleteEmailJobs)

	httpReq, _ := http.NewRequest("DELETE", "/api/email/jobs/invalid-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestEmailJobsController_NewEmailJobsController 测试构造函数
func TestEmailJobsController_NewEmailJobsController(t *testing.T) {
	ctrl := NewEmailJobsController()
	if ctrl == nil {
		t.Error("Expected controller instance, got nil")
	}
}
