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

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Pinger Redis 健康检查接口（避免强制依赖 go-redis 模块）
type Pinger interface {
	Ping(ctx context.Context) error
}

// inferenceStatus 返回推理栈（LLM 供应商）健康状态。
// 通过全局 ProviderFailover 的健康快照判断；未配置 failover 或无可用的供应商时返回 not_configured，
// 避免在没有推理能力的部署上误报不健康。
//
// P1 修复：此前健康检查完全不探测推理栈，LLM 挂掉时容器仍 healthy，
// 网关继续投流量导致全部 AI 回复失败。
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

// embeddingStatus 返回 embedding 服务可达性（TCP 层探测）。
//
// P1-6 新增：此前健康检查不探测 embedding 栈，RAG 向量化失败时无感知。
// 采用轻量 TCP dial（2s 超时）探测 BaseURL 的 host:port，不触发真实 embedding 计算，
// 避免每次健康探针产生昂贵的向量推理。
//
// 返回 "up" / "down" / "not_configured"。
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
	_ = conn.Close()
	return "up", ""
}

// HealthCheck 全维度健康检查
// 返回应用、数据库、Redis、推理栈 四层健康状态
func HealthCheck(redisClient Pinger, gormDB *gorm.DB) gin.HandlerFunc {
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
func ReadinessCheck(redisClient Pinger, gormDB *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Second)
		defer cancel()

		if database := gormDB; database == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not_ready",
				"reason": "database not initialized",
			})
			return
		}
		if err := gormDB.WithContext(ctx).Exec("SELECT 1").Error; err != nil {
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
		if infStatus := inferenceStatus(); infStatus == "down" {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not_ready",
				"reason": "inference: no healthy LLM provider",
			})
			return
		}
		if embStatus, embErr := embeddingStatus(); embStatus == "down" && os.Getenv("HEALTH_EMBEDDING_CRITICAL") == "true" {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not_ready",
				"reason": "embedding: " + embErr,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status":    "ready",
			"timestamp": time.Now().Unix(),
		})
	}
}

