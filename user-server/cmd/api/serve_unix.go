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
	// ListenAndServe 会创建 EndlessListener 后调 Serve;
	// 直接调 Serve 时 EndlessListener 为 nil → Accept 空指针 panic
	return srv.ListenAndServe()
}
