// langfuse_attrs.go Langfuse OTel 顶层属性写入（K-6）。
//
// Langfuse 支持 OpenTelemetry 接入，span 上的顶层属性 session_id / user_id / tags
// 用于其会话与用户维度的观测面板。本文件提供纯函数 ApplyLangfuseAttrs，
// 供各入口 span 统一打点；仓库现有 internal/pkg/tracing 为自研 DB sink，二者互不影响。
package tracing

import (
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Langfuse 顶层属性键（按约定使用下划线命名）
const (
	LangfuseSessionIDKey = "session_id"
	LangfuseUserIDKey    = "user_id"
	LangfuseTagsKey      = "tags"
)

// ApplyLangfuseAttrs 为 span 附加 Langfuse 顶层属性。
//   - sessionID/userID 去空白后非空才写入；
//   - tags 过滤空项后以单键多值属性写入，全空则不写 tags；
//   - span 为 nil 或未在记录态时直接忽略，不 panic。
func ApplyLangfuseAttrs(span trace.Span, sessionID, userID string, tags ...string) {
	if span == nil || !span.IsRecording() {
		return
	}
	if v := strings.TrimSpace(sessionID); v != "" {
		span.SetAttributes(attribute.String(LangfuseSessionIDKey, v))
	}
	if v := strings.TrimSpace(userID); v != "" {
		span.SetAttributes(attribute.String(LangfuseUserIDKey, v))
	}
	cleaned := make([]string, 0, len(tags))
	for _, tg := range tags {
		if v := strings.TrimSpace(tg); v != "" {
			cleaned = append(cleaned, v)
		}
	}
	if len(cleaned) > 0 {
		span.SetAttributes(attribute.StringSlice(LangfuseTagsKey, cleaned))
	}
}
