//go:build windows

package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func serveHTTP(addr string, r *gin.Engine) error {
	// 只设 ReadHeaderTimeout/IdleTimeout:不设 ReadTimeout(知识库大文件上传)与
	// WriteTimeout(SSE 长连接会被强制断开),慢速攻击面由头超时+各 handler 的
	// MaxBytesReader 兜底
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return srv.ListenAndServe()
}
