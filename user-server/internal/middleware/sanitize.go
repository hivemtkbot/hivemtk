
package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// PIIRule 单一脱敏规则
type PIIRule struct {
	FieldNames []string
	Pattern *regexp.Regexp
	Mask func(string) string
	Kind string
}

// 预定义脱敏规则
var (
	phoneRe = regexp.MustCompile(`1[3-9]\d{9}`)
	emailRe = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	idcardRe = regexp.MustCompile(`[1-9]\d{5}(?:18|19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]`)
	bankcardRe = regexp.MustCompile(`\b\d{16,19}\b`)
	tokenRe = regexp.MustCompile(`\b[A-Za-z0-9_-]{30,}\b`)
)

// DEFAULT_PII_RULES 默认脱敏规则
var DEFAULT_PII_RULES = []PIIRule{
	{
		FieldNames: []string{"password", "passwd", "pwd", "old_password", "new_password", "token", "secret", "api_key", "apikey", "access_token", "refresh_token", "authorization"},
		Kind:       "field",
		Mask:       func(s string) string { return "********" },
	},
	{
		Pattern: phoneRe,
		Kind:    "content",
		Mask: func(s string) string {
			if len(s) != 11 {
				return strings.Repeat("*", len(s))
			}
			return s[:3] + "****" + s[7:]
		},
	},
	{
		Pattern: emailRe,
		Kind:    "content",
		Mask: func(s string) string {
			at := strings.Index(s, "@")
			if at <= 1 {
				return "***" + s[at:]
			}
			return s[:1] + "***" + s[at:]
		},
	},
	{
		Pattern: idcardRe,
		Kind:    "content",
		Mask: func(s string) string {
			if len(s) < 8 {
				return strings.Repeat("*", len(s))
			}
			return s[:6] + strings.Repeat("*", len(s)-10) + s[len(s)-4:]
		},
	},
	{
		Pattern: bankcardRe,
		Kind:    "content",
		Mask: func(s string) string {
			if len(s) < 8 {
				return strings.Repeat("*", len(s))
			}
			return s[:4] + " **** **** " + s[len(s)-4:]
		},
	},
	{
		Pattern: tokenRe,
		Kind:    "content",
		Mask: func(s string) string {
			if len(s) <= 8 {
				return s
			}
			return s[:4] + "..." + s[len(s)-4:]
		},
	},
}


// SanitizeString 对字符串做内容级脱敏（手机/邮箱/身份证/银行卡/token）
// 返回脱敏后的字符串
func SanitizeString(s string) string {
	if s == "" {
		return s
	}
	for _, rule := range DEFAULT_PII_RULES {
		if rule.Kind != "content" || rule.Pattern == nil {
			continue
		}
		s = rule.Pattern.ReplaceAllStringFunc(s, rule.Mask)
	}
	return s
}

// SanitizeMap 对 map[string]any 做字段级 + 内容级脱敏（递归处理嵌套）
func SanitizeMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = sanitizeValue(k, v)
	}
	return out
}

// SanitizeJSON 对 JSON 字符串解析后脱敏再序列化
// 用于请求体 / 响应体的脱敏
func SanitizeJSON(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	// 尝试解析为 JSON object
	var any_ any
	if err := json.Unmarshal(data, &any_); err != nil {
		return []byte(SanitizeString(string(data)))
	}
	sanitized := sanitizeJSONValue("", any_)
	out, err := json.Marshal(sanitized)
	if err != nil {
		return []byte(SanitizeString(string(data)))
	}
	return out
}

// sanitizeValue 单一字段的脱敏（考虑字段名 + 值类型）
func sanitizeValue(key string, val any) any {
	lowerKey := strings.ToLower(key)
	for _, rule := range DEFAULT_PII_RULES {
		if rule.Kind != "field" {
			continue
		}
		for _, fn := range rule.FieldNames {
			if lowerKey == fn || strings.Contains(lowerKey, fn) {
				return rule.Mask(toString(val))
			}
		}
	}
	switch v := val.(type) {
	case string:
		return SanitizeString(v)
	case map[string]any:
		return SanitizeMap(v)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = sanitizeValue("", item)
		}
		return out
	}
	return val
}

// sanitizeJSONValue 处理 JSON 解码后的值（key 可能为空）
func sanitizeJSONValue(key string, val any) any {
	if m, ok := val.(map[string]any); ok {
		out := make(map[string]any, len(m))
		for k, v := range m {
			out[k] = sanitizeValue(k, v)
		}
		return out
	}
	return sanitizeValue(key, val)
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	if b, err := json.Marshal(v); err == nil {
		return string(b)
	}
	return ""
}


// SanitizeConfig 脱敏配置
type SanitizeConfig struct {
	SanitizeRequest bool
	SanitizeResponse bool
	MaxBodyBytes int64
}

// DefaultSanitizeConfig 默认配置
var DefaultSanitizeConfig = SanitizeConfig{
	SanitizeRequest:  true,
	SanitizeResponse: false, 
	MaxBodyBytes:     1 << 20, 
}

// SanitizeMiddleware 脱敏 Gin 中间件
// 用法：router.Use(middleware.SanitizeMiddleware())
// 注意：只对 application/json 类型生效；其他类型（multipart/form-data）需要单独处理
func SanitizeMiddleware() gin.HandlerFunc {
	return SanitizeMiddlewareWithConfig(DefaultSanitizeConfig)
}

// SanitizeMiddlewareWithConfig 自定义配置的脱敏中间件
func SanitizeMiddlewareWithConfig(cfg SanitizeConfig) gin.HandlerFunc {
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 1 << 20
	}
	return func(c *gin.Context) {
		if cfg.SanitizeRequest && isJSONContentType(c.GetHeader("Content-Type")) {
			body, err := io.ReadAll(io.LimitReader(c.Request.Body, cfg.MaxBodyBytes))
			if err == nil && len(body) > 0 {
				sanitized := SanitizeJSON(body)
				c.Request.Body = io.NopCloser(bytes.NewReader(sanitized))
				c.Request.ContentLength = int64(len(sanitized))
			}
		}
		c.Next()
	}
}

func isJSONContentType(ct string) bool {
	return strings.Contains(strings.ToLower(ct), "application/json")
}


var _bufPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

// SanitizeJSONPooled 使用 sync.Pool 优化的 JSON 脱敏（性能敏感场景）
func SanitizeJSONPooled(data []byte) []byte {
	buf := _bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer _bufPool.Put(buf)
	// 简易实现：先 unmarshal 再 marshal
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return []byte(SanitizeString(string(data)))
	}
	sanitized := sanitizeJSONValue("", v)
	if err := json.NewEncoder(buf).Encode(sanitized); err != nil {
		return []byte(SanitizeString(string(data)))
	}
	out := buf.Bytes()
	if len(out) > 0 && out[len(out)-1] == '\n' {
		out = out[:len(out)-1]
	}
	cp := make([]byte, len(out))
	copy(cp, out)
	return cp
}


