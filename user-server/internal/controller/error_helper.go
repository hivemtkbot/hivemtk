package controller

import (
	"errors"
	"net/http"
	"strings"

	"marketing/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// HandleDBError 处理服务层返回的错误。
// 如果错误表示记录不存在（gorm.ErrRecordNotFound 或消息包含"不存在"），
// 响应 404；否则响应 500 并附带操作描述。
// operation 参数用于构造 500 错误消息，如 "获取短链"、"创建活码"。
// 返回 true 表示已处理错误并写入响应，调用方应立即 return。
func HandleDBError(ctx *gin.Context, err error, operation string) bool {
	if err == nil {
		return false
	}
	if isNotFoundError(err) {
		response.NotFound(ctx, "记录不存在")
		return true
	}
	response.Error(ctx, http.StatusInternalServerError, operation+"失败: "+err.Error())
	return true
}

// HandleServiceError 处理服务层返回的业务错误。
// 与 HandleDBError 不同，非"记录不存在"的错误响应 400（业务校验错误），
// 适用于服务层会返回业务校验错误（如"系统模板不能修改"、"无权限修改"）的场景。
// 返回 true 表示已处理错误并写入响应，调用方应立即 return。
func HandleServiceError(ctx *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if isNotFoundError(err) {
		response.NotFound(ctx, "记录不存在")
		return true
	}
	response.Error(ctx, http.StatusBadRequest, err.Error())
	return true
}

// isNotFoundError 判断错误是否表示记录不存在
func isNotFoundError(err error) bool {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true
	}
	// 服务层可能使用 errors.New("xxx不存在") 包装错误，检查错误消息
	msg := err.Error()
	return strings.Contains(msg, "不存在") || strings.Contains(msg, "not found")
}

// IsNotFoundError 公开版 isNotFoundError(供跨包调用)
func IsNotFoundError(err error) bool {
	return isNotFoundError(err)
}
