package response

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/pkg/utils"

	"github.com/gin-gonic/gin"
)

func setupTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest("GET", "http://test.com", nil)
	ctx.Request = req
	return ctx, w
}

func TestSuccess(t *testing.T) {
	ctx, w := setupTestContext()

	data := map[string]string{"key": "value"}
	Success(ctx, data, "success")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestError(t *testing.T) {
	ctx, w := setupTestContext()

	Error(ctx, utils.ErrorCodeInvalidParameter, "bad request")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestErrorWithDetails(t *testing.T) {
	ctx, w := setupTestContext()

	Error(ctx, utils.ErrorCodeInvalidParameter, "bad request", "detail info")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestErrorWithLog(t *testing.T) {
	ctx, w := setupTestContext()

	ErrorWithLog(ctx, utils.ErrorCodeInvalidParameter, "bad request")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestValidationError(t *testing.T) {
	ctx, w := setupTestContext()

	ValidationError(ctx, "validation error", "field is required")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestDatabaseError(t *testing.T) {
	ctx, w := setupTestContext()

	DatabaseError(ctx, "db error")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}
}

func TestBusinessError(t *testing.T) {
	ctx, w := setupTestContext()

	BusinessError(ctx, "business error", "invalid state")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestAuthError(t *testing.T) {
	ctx, w := setupTestContext()

	AuthError(ctx, "unauthorized")

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestSuccessWithList(t *testing.T) {
	ctx, w := setupTestContext()

	data := []map[string]string{{"key": "value"}}
	SuccessWithList(ctx, data, 100)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestSuccessWithPage(t *testing.T) {
	ctx, w := setupTestContext()

	data := []map[string]string{{"key": "value"}}
	SuccessWithPage(ctx, data, 1, 10, 100)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestErrorWithCode(t *testing.T) {
	ctx, w := setupTestContext()

	ErrorWithCode(ctx, utils.ErrorCodeInvalidParameter, http.StatusBadRequest, "custom error")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestNotFoundError(t *testing.T) {
	ctx, w := setupTestContext()

	NotFoundError(ctx, "user")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestNotFoundErrorEmptyResource(t *testing.T) {
	ctx, w := setupTestContext()

	NotFoundError(ctx, "")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestInvalidParameterError(t *testing.T) {
	ctx, w := setupTestContext()

	InvalidParameterError(ctx, "email", "邮箱格式不正确")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestOperationFailedError(t *testing.T) {
	ctx, w := setupTestContext()

	OperationFailedError(ctx, "创建订单")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}
}

func TestOperationFailedErrorEmptyOperation(t *testing.T) {
	ctx, w := setupTestContext()

	OperationFailedError(ctx, "")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}
}

func TestFileUploadError(t *testing.T) {
	ctx, w := setupTestContext()

	FileUploadError(ctx, utils.ErrorCodeUploadFailed, "文件上传失败")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}
}

func TestFileUploadErrorEmptyCode(t *testing.T) {
	ctx, w := setupTestContext()

	FileUploadError(ctx, "", "文件上传失败")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}
}

// TestError_EmptyMessage tests Error with empty message to trigger default message lookup
func TestError_EmptyMessage(t *testing.T) {
	ctx, w := setupTestContext()

	Error(ctx, utils.ErrorCodeInvalidParameter, "")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestErrorWithLog_WithDetails tests ErrorWithLog with details to cover the details branch
func TestErrorWithLog_WithDetails(t *testing.T) {
	ctx, w := setupTestContext()

	ErrorWithLog(ctx, utils.ErrorCodeInvalidParameter, "error with details", "detail info")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestError_IntCode(t *testing.T) {
	ctx, w := setupTestContext()

	Error(ctx, http.StatusServiceUnavailable, "service unavailable")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}
}

func TestError_UnknownCode(t *testing.T) {
	ctx, w := setupTestContext()

	Error(ctx, "unknown_type", "unknown error")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}
}

func TestError_WithDetailData(t *testing.T) {
	ctx, w := setupTestContext()

	detailData := map[string]string{"field": "email", "reason": "invalid format"}
	Error(ctx, utils.ErrorCodeInvalidParameter, "bad request", detailData)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestErrorCodeFromHTTPCode tests all branches in errorCodeFromHTTPCode
func TestErrorCodeFromHTTPCode(t *testing.T) {
	tests := []struct {
		httpCode     int
		expectedCode utils.ErrorCode
		name         string
	}{
		{http.StatusBadRequest, utils.ErrorCodeInvalidParameter, "400"},
		{http.StatusUnauthorized, utils.ErrorCodeUnauthorized, "401"},
		{http.StatusForbidden, utils.ErrorCodeForbidden, "403"},
		{http.StatusNotFound, utils.ErrorCodeNotFound, "404"},
		{http.StatusConflict, utils.ErrorCodeDuplicateEntry, "409"},
		{http.StatusTooManyRequests, utils.ErrorCodeInsufficientQuota, "429"},
		{http.StatusInternalServerError, utils.ErrorCodeInternalError, "500"},
		{http.StatusServiceUnavailable, utils.ErrorCodeServiceUnavailable, "503"},
		{http.StatusGatewayTimeout, utils.ErrorCodeUnknown, "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := errorCodeFromHTTPCode(tt.httpCode)
			if result != tt.expectedCode {
				t.Errorf("errorCodeFromHTTPCode(%d) = %v, want %v", tt.httpCode, result, tt.expectedCode)
			}
		})
	}
}

