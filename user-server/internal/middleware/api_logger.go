package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"marketing/internal/pkg/utils/logger"
)

const (
	apiLogReqBodyCap  = 8 * 1024  // 请求体记录上限
	apiLogRespBodyCap = 16 * 1024 // 响应体记录上限
)

// apiLogSkipPrefixes 不记录交互数据的路径（健康检查/指标等噪声端点）。
var apiLogSkipPrefixes = []string{"/health", "/healthz", "/readyz", "/metrics", "/swagger"}

// apiLogSensitiveKeys 请求体/query 中需要脱敏的字段（小写匹配）。
var apiLogSensitiveKeys = map[string]struct{}{
	"password":                        {},
	"passwd":                          {},
	"token":                           {},
	"secret":                          {},
	"authorization":                   {},
	"access_token":                    {},
	"refresh_token":                   {},
	"api_key":                         {},
	"apikey":                          {},
	"jwt":                             {},
	"secret_token":                    {},
	"x-telegram-bot-api-secret-token": {},
}

// APIInteractionLogger 记录每个 /api 请求的交互数据（请求 + 响应）。
//
// 行为：
//   - 仅对 /api 路径生效，健康检查等噪声端点跳过。
//   - 读取并记录请求体（随后还原，handler 仍可读到），敏感字段自动脱敏。
//   - 包装响应写入器捕获响应体；SSE/流式响应自动跳过，其余按上限截断。
//   - 状态码 >=500 记 Error、>=400 记 Warn、其余记 Info，事件名 api_interaction。
//
// 注册顺序：须在 TraceMiddleware 之后，使日志自动携带 trace_id。
func APIInteractionLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if !strings.HasPrefix(path, "/api") {
			c.Next()
			return
		}
		for _, p := range apiLogSkipPrefixes {
			if strings.HasPrefix(path, p) {
				c.Next()
				return
			}
		}

		// 以有界缓冲（TeeReader）在 handler 消费 body 时旁路捕获，避免对超大请求体执行
		// io.ReadAll 导致整请求体常驻内存（内存放大 / DoS 风险）。捕获上限 apiLogReqBodyCap
		// 同时作为日志截断上限；handler 仍读取原始 body 流，绝不被截断。
		var reqCap *cappedBufferWriter
		if c.Request.Body != nil {
			reqCap = &cappedBufferWriter{cap: apiLogReqBodyCap}
			c.Request.Body = io.NopCloser(io.TeeReader(c.Request.Body, reqCap))
		}

		// 包装响应写入器以捕获响应体
		w := &apiLogResponseWriter{ResponseWriter: c.Writer, cap: apiLogRespBodyCap}
		c.Writer = w

		start := time.Now()
		c.Next()
		latencyMs := time.Since(start).Milliseconds()

		reqBodyLog := ""
		if reqCap != nil {
			reqBodyLog = redactJSON(reqCap.Bytes(), apiLogReqBodyCap)
		}

		status := c.Writer.Status()
		respBodyLog := w.snapshot()

		log := logger.Ctx(c.Request.Context())
		var ev *zerolog.Event
		switch {
		case status >= 500:
			ev = log.Error()
		case status >= 400:
			ev = log.Warn()
		default:
			ev = log.Info()
		}
		ev = ev.Str("event", "api_interaction").
			Str("method", c.Request.Method).
			Str("path", path).
			Int("status", status).
			Int64("latency_ms", latencyMs).
			Str("client_ip", c.ClientIP()).
			Str("request_body", reqBodyLog).
			Str("response_body", respBodyLog)
		if rawQuery := c.Request.URL.RawQuery; rawQuery != "" {
			ev = ev.Str("query", redactQuery(c.Request.URL.Query()))
		}
		if len(c.Errors) > 0 {
			ev = ev.Str("gin_errors", c.Errors.String())
		}
		ev.Msg("api_interaction")
	}
}

// apiLogResponseWriter 包装 gin.ResponseWriter，在透写的同时缓冲响应体（受上限约束）。
// 对 SSE/流式 Content-Type 自动跳过缓冲，避免内存膨胀与破坏流式传输。
type apiLogResponseWriter struct {
	gin.ResponseWriter
	buf   bytes.Buffer
	cap   int
	full  bool
	skip  bool
	wrote bool
}

func (w *apiLogResponseWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.wrote = true
		if ct := w.Header().Get("Content-Type"); ct != "" {
			if strings.Contains(ct, "text/event-stream") ||
				strings.Contains(ct, "application/x-ndjson") ||
				strings.Contains(ct, "application/octet-stream") {
				w.skip = true
			}
		}
	}
	if w.skip {
		return w.ResponseWriter.Write(b)
	}
	if !w.full {
		if w.buf.Len()+len(b) > w.cap {
			if remain := w.cap - w.buf.Len(); remain > 0 {
				w.buf.Write(b[:remain])
			}
			w.full = true
		} else {
			w.buf.Write(b)
		}
	}
	return w.ResponseWriter.Write(b)
}

func (w *apiLogResponseWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

// snapshot 返回捕获到的响应体快照；流式响应返回提示文本。
func (w *apiLogResponseWriter) snapshot() string {
	if w.skip {
		return "(streaming response, not captured)"
	}
	s := w.buf.String()
	if w.full {
		s += "...(truncated)"
	}
	return s
}

// cappedBufferWriter 有界写入器：最多保留 cap 字节，超出部分静默丢弃。
// 用于 TeeReader 旁路捕获请求体，保证内存占用上限，杜绝超大请求体 OOM。
type cappedBufferWriter struct {
	buf  bytes.Buffer
	cap  int
	full bool
}

func (w *cappedBufferWriter) Write(p []byte) (int, error) {
	if w.full {
		return len(p), nil
	}
	if w.buf.Len()+len(p) > w.cap {
		if remain := w.cap - w.buf.Len(); remain > 0 {
			w.buf.Write(p[:remain])
		}
		w.full = true
		return len(p), nil
	}
	w.buf.Write(p)
	return len(p), nil
}

func (w *cappedBufferWriter) Bytes() []byte { return w.buf.Bytes() }

// redactJSON 解析 JSON 并脱敏敏感字段；非 JSON 或超限则截断后原样返回。
func redactJSON(b []byte, capSize int) string {
	if len(b) == 0 {
		return ""
	}
	if len(b) > capSize {
		return string(b[:capSize]) + "...(truncated)"
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return string(b)
	}
	redactValue(v)
	out, err := json.Marshal(v)
	if err != nil {
		return string(b)
	}
	if len(out) > capSize {
		return string(out[:capSize]) + "...(truncated)"
	}
	return string(out)
}

// redactValue 递归脱敏 map/array 中的敏感字段（原地修改）。
func redactValue(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if _, ok := apiLogSensitiveKeys[strings.ToLower(k)]; ok {
				t[k] = "***REDACTED***"
			} else {
				redactValue(val)
			}
		}
	case []any:
		for _, item := range t {
			redactValue(item)
		}
	}
}

// redactQuery 返回脱敏后的 query 字符串。
func redactQuery(q url.Values) string {
	clone := url.Values{}
	for k, vs := range q {
		if _, ok := apiLogSensitiveKeys[strings.ToLower(k)]; ok {
			clone[k] = []string{"***REDACTED***"}
		} else {
			clone[k] = vs
		}
	}
	return clone.Encode()
}
