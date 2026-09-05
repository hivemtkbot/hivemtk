package router

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/app"
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Pinger Redis 健康检查接口（避免强制依赖 go-redis 模块）
type Pinger interface {
	Ping(ctx context.Context) error
}

func inferenceStatus() string {
	f := app.GetGlobalProviderFailover()
	if f == nil {
		return "not_configured"
	}
	all := f.GetAllHealth()
	if len(all) == 0 {
		return "not_configured"
	}
	for _, h := range all {
		if h.Status == llm.ProviderStatusUp {
			return "up"
		}
	}
	return "down"
}

func embeddingStatus() (string, string) {
	svc := llm.NewEmbeddingService()
	if svc == nil {
		return "not_configured", ""
	}
	base := svc.DefaultConfig().BaseURL
	if base == "" {
		return "not_configured", ""
	}
	u, err := url.Parse(base)
	if err != nil {
		return "down", "invalid embedding base url: " + err.Error()
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "8208"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		return "down", "embedding unreachable: " + err.Error()
	}
	utils.WarnErrKV("router.health.embedClose", conn.Close(),
		"host", host, "port", port)
	return "up", ""
}

// HealthCheck 全维度健康检查
// 返回应用、数据库、Redis、推理栈 四层健康状态
// 统一响应格式: {code:0, message:"ok", data:{status,checks,...}}
func HealthCheck(redisClient Pinger, gormDB *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		checks := gin.H{}
		overallOK := true

		dbStatus := "ok"
		dbErr := ""
		if database := gormDB; database != nil {
			if err := database.WithContext(ctx).Exec("SELECT 1").Error; err != nil {
				dbStatus = "down"
				dbErr = err.Error()
				overallOK = false
			}
		} else {
			dbStatus = "not_configured"
		}
		checks["database"] = gin.H{"status": dbStatus, "error": dbErr}

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

		infStatus := inferenceStatus()
		infErr := ""
		if infStatus == "down" {
			overallOK = false
			infErr = "no healthy LLM provider"
		}
		checks["inference"] = gin.H{"status": infStatus, "error": infErr}

		embStatus, embErr := embeddingStatus()
		checks["embedding"] = gin.H{"status": embStatus, "error": embErr}
		if embStatus == "down" && os.Getenv("HEALTH_EMBEDDING_CRITICAL") == "true" {
			overallOK = false
		}

		data := gin.H{
			"status":    "ok",
			"timestamp": time.Now().Unix(),
			"service":   "user-server",
			"checks":    checks,
		}
		if !overallOK {
			data["status"] = "degraded"
			response.ErrorWithBusinessCode(c, 50301, "service degraded", data)
			return
		}
		response.Success(c, data, "ok")
	}
}

// LivenessCheck 存活性检查（轻量级，不检查依赖）
func LivenessCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		response.Success(c, gin.H{
			"status":    "alive",
			"timestamp": time.Now().Unix(),
		}, "ok")
	}
}

func notReadyResponse(c *gin.Context, code int, reason string) {
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"code":    code,
		"message": reason,
		"data": gin.H{
			"status": "not_ready",
			"reason": reason,
		},
	})
}

// ReadinessCheck 就绪性检查（检查依赖）
func ReadinessCheck(redisClient Pinger, gormDB *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Second)
		defer cancel()

		if database := gormDB; database == nil {
			notReadyResponse(c, 50302, "database not initialized")
			return
		}
		if err := gormDB.WithContext(ctx).Exec("SELECT 1").Error; err != nil {
			notReadyResponse(c, 50303, "database: "+err.Error())
			return
		}
		if redisClient != nil {
			if err := redisClient.Ping(ctx); err != nil {
				notReadyResponse(c, 50304, "redis: "+err.Error())
				return
			}
		}
		if infStatus := inferenceStatus(); infStatus == "down" {
			notReadyResponse(c, 50305, "inference: no healthy LLM provider")
			return
		}
		if embStatus, embErr := embeddingStatus(); embStatus == "down" && os.Getenv("HEALTH_EMBEDDING_CRITICAL") == "true" {
			notReadyResponse(c, 50306, "embedding: "+embErr)
			return
		}
		response.Success(c, gin.H{
			"status":    "ready",
			"timestamp": time.Now().Unix(),
		}, "ok")
	}
}
