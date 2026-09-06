//go:build windows

package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func serveHTTP(addr string, r *gin.Engine) error {
	return http.ListenAndServe(addr, r)
}
