//go:build unix

package main

import (
	"time"

	"github.com/fvbock/endless"
	"github.com/gin-gonic/gin"
)

func serveHTTP(addr string, r *gin.Engine) error {
	// 只设 ReadHeaderTimeout/IdleTimeout:不设 ReadTimeout(知识库大文件上传)与
	// WriteTimeout(SSE 长连接会被强制断开);endless.Server 内嵌 http.Server,
	// 热重启能力与超时配置并存
	srv := endless.NewServer(addr, r)
	srv.ReadHeaderTimeout = 10 * time.Second
	srv.IdleTimeout = 120 * time.Second
	return srv.Serve()
}
