//go:build unix

package main

import (
	"github.com/fvbock/endless"
	"github.com/gin-gonic/gin"
)

func serveHTTP(addr string, r *gin.Engine) error {
	return endless.ListenAndServe(addr, r)
}
