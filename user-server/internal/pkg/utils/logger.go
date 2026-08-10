package utils

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"hivemtk-user/internal/pkg/utils/logger"
)

// 本文件是「统一日志系统」的兼容层：原 utils 自带的 Logger 实现已统一委托给
// pkg/utils/logger（基于 zerolog 的全局日志器）。这样项目内无论调用
// logger.Info / utils.Info / utils.LogErrorWithRequest，最终都走到同一套
// 级别/格式/落盘/trace 透传逻辑，避免多套互不关联的日志实现。

// LogLevel 日志级别（兼容旧调用方）。
type LogLevel string

const (
	DEBUG LogLevel = "debug"
	INFO  LogLevel = "info"
	WARN  LogLevel = "warn"
	ERROR LogLevel = "error"
)

// InitLogger 兼容旧签名：debug=true 映射到 debug 级，传入 logFile 时改为 both 输出。
func InitLogger(level LogLevel, logFile string) {
	cfg := logger.LoggingConfig{Level: string(level), Format: "console", Output: "stdout"}
	if logFile != "" {
		cfg.Output = "both"
		cfg.File = logFile
	}
	logger.InitLogger(cfg)
}

// GetLogger 返回统一全局日志器（*zerolog.Logger）。
func GetLogger() *zerolog.Logger {
	return logger.GetLogger()
}

// Logger 兼容旧调用方的轻量门面；方法均转发到统一日志器。
type Logger struct{}

var defaultLogger = &Logger{}

// NewLogger 返回 Logger 门面（单例）。
func NewLogger() *Logger { return defaultLogger }

// Debug 记录 debug 级日志。
func (l *Logger) Debug(format string, args ...any) { logger.Debugf(format, args...) }

// Info 记录 info 级日志。
func (l *Logger) Info(format string, args ...any) { logger.Infof(format, args...) }

// Warn 记录 warn 级日志。
func (l *Logger) Warn(format string, args ...any) { logger.Warnf(format, args...) }

// Error 记录 error 级日志。
func (l *Logger) Error(format string, args ...any) { logger.Errorf(format, args...) }

// Write 实现 io.Writer，便于将第三方库的日志重定向到统一日志器。
func (l *Logger) Write(p []byte) (int, error) { logger.Info(string(p)); return len(p), nil }

// WriteString 实现字符串写入。
func (l *Logger) WriteString(s string) (int, error) { logger.Info(s); return len(s), nil }

// Debug 包级便捷方法。
func Debug(format string, args ...any) { logger.Debugf(format, args...) }

// Info 包级便捷方法。
func Info(format string, args ...any) { logger.Infof(format, args...) }

// Warn 包级便捷方法。
func Warn(format string, args ...any) { logger.Warnf(format, args...) }

// Error 包级便捷方法。
func Error(format string, args ...any) { logger.Errorf(format, args...) }

// GinLogger 兼容旧 Gin 中间件签名；新代码请使用 middleware.TraceMiddleware。
func GinLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

// LogErrorWithRequest 统一记录带请求上下文（含 trace_id）的错误。
// 由 pkg/utils/response 在统一错误响应中调用。
func LogErrorWithRequest(c *gin.Context, err error) error {
	logger.Ctx(c.Request.Context()).Error().
		Err(err).
		Str("module", "http").
		Str("method", c.Request.Method).
		Str("path", c.FullPath()).
		Str("client_ip", c.ClientIP()).
		Msg("request error")
	return err
}

// LogRequest 兼容旧调用：记录入站请求。
func LogRequest(method, path, ip string) {
	logger.Infof("%s %s from %s", method, path, ip)
}

// LogResponse 兼容旧调用：记录出站响应与耗时。
func LogResponse(status int, path string, cost time.Duration) {
	logger.Infof("%s %d cost=%s", path, status, cost)
}
