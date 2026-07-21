package middleware

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"marketing/internal/pkg/utils/logger"
)

// 敏感字段正则表达式
var (
	// 匹配密码字段
	passwordRegex = regexp.MustCompile(`(?i)"password"\s*:\s*"[^"]*"`)
	// 匹配手机号（中国）
	phoneRegex = regexp.MustCompile(`1[3-9]\d{9}`)
	// 匹配身份证号
	idCardRegex = regexp.MustCompile(`[1-9]\d{5}(18|19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]`)
	// 匹配银行卡号
	bankCardRegex = regexp.MustCompile(`[62]\d{13,16}`)
	// 匹配邮箱
	emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	// 匹配 Authorization header
	authHeaderRegex = regexp.MustCompile(`Bearer\s+\S+`)
	// 匹配 API Key
	apiKeyRegex = regexp.MustCompile(`(?i)api[_-]?key\s*[:=]\s*\S+`)
	// 匹配 JWT Token
	jwtRegex = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)
	// 匹配私钥内容
	privateKeyRegex = regexp.MustCompile(`-----BEGIN\s+(?:RSA\s+)?PRIVATE\s+KEY-----[\s\S]*?-----END\s+(?:RSA\s+)?PRIVATE\s+KEY-----`)
)

// 脱敏掩码
const (
	maskFull     = "******"
	maskPartial4 = "****"
	maskPhone    = "****"
	maskIDCard   = "*****************"
)

// SensitiveLogConfig 日志脱敏配置
type SensitiveLogConfig struct {
	// 是否脱敏密码
	MaskPassword bool
	// 是否脱敏手机号
	MaskPhone bool
	// 是否脱敏身份证
	MaskIDCard bool
	// 是否脱敏银行卡
	MaskBankCard bool
	// 是否脱敏邮箱
	MaskEmail bool
	// 是否脱敏 Authorization
	MaskAuthHeader bool
	// 是否脱敏 API Key
	MaskAPIKey bool
	// 是否脱敏 JWT
	MaskJWT bool
	// 是否脱敏私钥
	MaskPrivateKey bool
	// 是否跳过某些路径
	SkipPaths []string
}

// DefaultSensitiveLogConfig 默认脱敏配置
var DefaultSensitiveLogConfig = SensitiveLogConfig{
	MaskPassword:   true,
	MaskPhone:      true,
	MaskIDCard:     true,
	MaskBankCard:   true,
	MaskEmail:      false, // 默认不脱敏邮箱，便于排查问题
	MaskAuthHeader: true,
	MaskAPIKey:     true,
	MaskJWT:        true,
	MaskPrivateKey: true,
	SkipPaths:      []string{"/api/health", "/api/system/info"},
}

// skipPath 检查路径是否跳过脱敏
func (c *SensitiveLogConfig) skipPath(path string) bool {
	for _, skip := range c.SkipPaths {
		if path == skip {
			return true
		}
	}
	return false
}

// maskString 脱敏字符串
func maskString(s string, keepPrefix, keepSuffix int) string {
	if len(s) <= keepPrefix+keepSuffix {
		return maskFull
	}
	return s[:keepPrefix] + maskFull + s[len(s)-keepSuffix:]
}

// desensitize 对字符串进行脱敏
func (c *SensitiveLogConfig) desensitize(s string) string {
	// 脱敏密码
	if c.MaskPassword {
		s = passwordRegex.ReplaceAllString(s, `"password":"${maskFull}"`)
	}

	// 脱敏手机号
	if c.MaskPhone {
		s = phoneRegex.ReplaceAllStringFunc(s, func(phone string) string {
			return maskString(phone, 3, 4)
		})
	}

	// 脱敏身份证
	if c.MaskIDCard {
		s = idCardRegex.ReplaceAllString(s, `"id_card":"${maskIDCard}"`)
	}

	// 脱敏银行卡
	if c.MaskBankCard {
		s = bankCardRegex.ReplaceAllStringFunc(s, func(card string) string {
			return maskString(card, 4, 4)
		})
	}

	// 脱敏邮箱
	if c.MaskEmail {
		s = emailRegex.ReplaceAllStringFunc(s, func(email string) string {
			parts := strings.Split(email, "@")
			if len(parts) == 2 {
				return maskString(parts[0], 2, 0) + "@" + parts[1]
			}
			return email
		})
	}

	// 脱敏 Authorization Header
	if c.MaskAuthHeader {
		s = authHeaderRegex.ReplaceAllString(s, "Bearer "+maskFull)
	}

	// 脱敏 API Key
	if c.MaskAPIKey {
		s = apiKeyRegex.ReplaceAllStringFunc(s, func(match string) string {
			parts := strings.SplitN(match, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[0]) + ": " + maskFull
			}
			return match
		})
	}

	// 脱敏 JWT Token
	if c.MaskJWT {
		s = jwtRegex.ReplaceAllString(s, maskFull)
	}

	// 脱敏私钥
	if c.MaskPrivateKey {
		s = privateKeyRegex.ReplaceAllString(s, maskFull)
	}

	return s
}

// SensitiveLogMiddleware 日志脱敏中间件
// 对请求和响应日志中的敏感信息进行脱敏处理
func SensitiveLogMiddleware(config ...SensitiveLogConfig) gin.HandlerFunc {
	cfg := DefaultSensitiveLogConfig
	if len(config) > 0 {
		cfg = config[0]
	}

	return func(c *gin.Context) {
		// 跳过脱敏的路径
		if cfg.skipPath(c.Request.URL.Path) {
			c.Next()
			return
		}

		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// 处理请求
		c.Next()

		// 计算处理时间
		latency := time.Since(start)

		// 获取状态码
		statusCode := c.Writer.Status()

		// 构建日志消息
		clientIP := c.ClientIP()
		method := c.Request.Method
		errorMessage := c.Errors.String()

		if raw != "" {
			path = path + "?" + raw
		}

		// 脱敏路径中的敏感信息
		safePath := cfg.desensitize(path)

		// 记录日志（统一日志器：自动携带 trace_id 与 module=http）
		if len(c.Errors) > 0 {
			logger.Ctx(c.Request.Context()).Error().
				Str("ip", clientIP).
				Str("method", method).
				Str("path", safePath).
				Int("status", statusCode).
				Dur("latency", latency).
				Str("error", errorMessage).
				Str("module", "http").
				Msg("request error")
		}
	}
}

// DesensitizeHeaders 对请求头进行脱敏处理（用于调试输出）
func DesensitizeHeaders(headers http.Header, config ...SensitiveLogConfig) http.Header {
	cfg := DefaultSensitiveLogConfig
	if len(config) > 0 {
		cfg = config[0]
	}

	// 复制 headers
	safeHeaders := make(http.Header)
	for k, v := range headers {
		safeHeaders[k] = v
	}

	// 脱敏敏感头
	if cfg.MaskAuthHeader {
		if auth := safeHeaders.Get("Authorization"); auth != "" {
			safeHeaders.Set("Authorization", "Bearer "+maskFull)
		}
	}

	if cfg.MaskAPIKey {
		if apiKey := safeHeaders.Get("X-API-KEY"); apiKey != "" {
			safeHeaders.Set("X-API-KEY", maskFull)
		}
	}

	return safeHeaders
}

// DesensitizeString 对外提供脱敏工具函数
func DesensitizeString(s string, config ...SensitiveLogConfig) string {
	cfg := DefaultSensitiveLogConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	return cfg.desensitize(s)
}
