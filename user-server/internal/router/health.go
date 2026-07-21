package router

import (
	"context"
	"net/http"
	"time"

	"marketing/internal/pkg/utils/db"

	"github.com/gin-gonic/gin"
)

// Pinger Redis 健康检查接口（避免强制依赖 go-redis 模块）
type Pinger interface {
	Ping(ctx context.Context) error
}

// HealthCheck 全维度健康检查
// 返回应用、数据库、Redis 三层健康状态
func HealthCheck(redisClient Pinger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		result := gin.H{
			"status":    "ok",
			"timestamp": time.Now().Unix(),
			"service":   "user-server",
			"checks":    gin.H{},
		}
		checks := result["checks"].(gin.H)
		overallOK := true

		// 1. 数据库健康检查
		dbStatus := "ok"
		dbErr := ""
		if database := db.GetDB(); database != nil {
			if err := database.WithContext(ctx).Exec("SELECT 1").Error; err != nil {
				dbStatus = "down"
				dbErr = err.Error()
				overallOK = false
			}
		} else {
			dbStatus = "not_configured"
		}
		checks["database"] = gin.H{"status": dbStatus, "error": dbErr}

		// 2. Redis 健康检查
		redisStatus := "not_configured"
		redisErr := ""
		if redisClient != nil {
			redisStatus = "ok"
			if err := redisClient.Ping(ctx); err != nil {
				redisStatus = "down"
				redisErr = err.Error()
				overallOK = false
			}
		}
		checks["redis"] = gin.H{"status": redisStatus, "error": redisErr}

		if !overallOK {
			result["status"] = "degraded"
			c.JSON(http.StatusServiceUnavailable, result)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

// LivenessCheck 存活性检查（轻量级，不检查依赖）
func LivenessCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "alive",
			"timestamp": time.Now().Unix(),
		})
	}
}

// ReadinessCheck 就绪性检查（检查依赖）
func ReadinessCheck(redisClient Pinger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Second)
		defer cancel()

		if database := db.GetDB(); database == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not_ready",
				"reason": "database not initialized",
			})
			return
		}
		if err := db.GetDB().WithContext(ctx).Exec("SELECT 1").Error; err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not_ready",
				"reason": "database: " + err.Error(),
			})
			return
		}
		if redisClient != nil {
			if err := redisClient.Ping(ctx); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"status": "not_ready",
					"reason": "redis: " + err.Error(),
				})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"status":    "ready",
			"timestamp": time.Now().Unix(),
		})
	}
}
