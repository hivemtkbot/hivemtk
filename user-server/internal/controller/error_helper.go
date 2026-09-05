package controller

import (
	"hivemtk-user/internal/pkg/errhttp"

	"github.com/gin-gonic/gin"
)

func HandleDBError(ctx *gin.Context, err error, operation string) bool {
	return errhttp.HandleDBError(ctx, err, operation)
}

// HandleServiceError 处理服务层返回的业务错误（委托 pkg/errhttp）。
func HandleServiceError(ctx *gin.Context, err error) bool {
	return errhttp.HandleServiceError(ctx, err)
}

func isNotFoundError(err error) bool {
	return errhttp.IsNotFoundError(err)
}

// IsNotFoundError 公开版 isNotFoundError(供跨包调用)
func IsNotFoundError(err error) bool {
	return isNotFoundError(err)
}
