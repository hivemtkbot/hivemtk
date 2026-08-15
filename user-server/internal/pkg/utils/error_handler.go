package utils

import (
	"fmt"
	"runtime"
	"time"

	"hivemtk-user/internal/pkg/utils/logger"

	"github.com/gin-gonic/gin"
)

// ErrorType 表示错误类型
type ErrorType string

const (
	ErrorTypeValidation ErrorType = "validation_error"
	ErrorTypeDatabase   ErrorType = "database_error"
	ErrorTypeBusiness   ErrorType = "business_error"
	ErrorTypeSystem     ErrorType = "system_error"
	ErrorTypeAuth       ErrorType = "auth_error"
)

// AppError 应用程序错误结构
type AppError struct {
	Type       ErrorType `json:"type"`
	Message    string    `json:"message"`
	Code       int       `json:"code"`
	Details    string    `json:"details,omitempty"`
	StackTrace string    `json:"stack_trace,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// Error 实现 error 接口
func (e *AppError) Error() string {
	return fmt.Sprintf("[%s] %s: %s", e.Type, e.Message, e.Details)
}

// NewAppError 创建一个新的应用程序错误
func NewAppError(errorType ErrorType, message string, code int, details string) *AppError {
	return &AppError{
		Type:      errorType,
		Message:   message,
		Code:      code,
		Details:   details,
		Timestamp: time.Now(),
	}
}

// LogError 记录错误日志
func LogError(err error, ctx *gin.Context) {
	var appErr *AppError
	if ae, ok := err.(*AppError); ok {
		appErr = ae
	} else {
		appErr = &AppError{
			Type:      ErrorTypeSystem,
			Message:   "系统错误",
			Code:      500,
			Details:   err.Error(),
			Timestamp: time.Now(),
		}
	}

	stackBuf := make([]byte, 4096)
	n := runtime.Stack(stackBuf, false)
	stackTrace := string(stackBuf[:n])

	appErr.StackTrace = stackTrace

	logger.Errorf("[ERROR] Type: %s, Code: %d, Message: %s, Details: %s, Time: %s, Trace: %s",
		appErr.Type, appErr.Code, appErr.Message, appErr.Details, appErr.Timestamp.Format(time.RFC3339), stackTrace)

	if ctx != nil {
		logger.Errorf("[CONTEXT] Method: %s, Path: %s, ClientIP: %s", ctx.Request.Method, ctx.Request.URL.Path, ctx.ClientIP())
	}
}

// HandleError 统一处理错误并返回响应
func HandleError(ctx *gin.Context, err error) {
	var appErr *AppError
	if ae, ok := err.(*AppError); ok {
		appErr = ae
	} else {
		appErr = &AppError{
			Type:      ErrorTypeSystem,
			Message:   "系统错误",
			Code:      500,
			Details:   err.Error(),
			Timestamp: time.Now(),
		}
	}

	LogError(appErr, ctx)

	ctx.JSON(appErr.Code, gin.H{
		"code":      appErr.Code,
		"message":   appErr.Message,
		"details":   appErr.Details,
		"type":      appErr.Type,
		"timestamp": appErr.Timestamp.Format(time.RFC3339),
	})
}

// HandleValidationError 处理验证错误
func HandleValidationError(ctx *gin.Context, message string, details string) {
	err := NewAppError(ErrorTypeValidation, message, 400, details)
	HandleError(ctx, err)
}

// HandleDatabaseError 处理数据库错误
func HandleDatabaseError(ctx *gin.Context, details string) {
	err := NewAppError(ErrorTypeDatabase, "数据库操作失败", 500, details)
	HandleError(ctx, err)
}

// HandleBusinessError 处理业务错误
func HandleBusinessError(ctx *gin.Context, message string, details string) {
	err := NewAppError(ErrorTypeBusiness, message, 400, details)
	HandleError(ctx, err)
}

// HandleAuthError 处理认证错误
func HandleAuthError(ctx *gin.Context, message string) {
	err := NewAppError(ErrorTypeAuth, message, 401, "")
	HandleError(ctx, err)
}

