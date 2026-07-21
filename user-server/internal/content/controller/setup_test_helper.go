package controller

import "github.com/gin-gonic/gin"

// setupGinEngine 设置 Gin 引擎(测试用)
func setupGinEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}
