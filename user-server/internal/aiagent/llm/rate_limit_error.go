package llm

import (
	"fmt"
	"strconv"
	"time"
)

// RateLimitError 429 限流的结构化错误（D05）。
//
// 业界契约（对齐 LiteLLM cooldown_handlers / OpenRouter 文档）：
//   - 429 单次即冷却，绕过连续失败计数（限流不是健康问题，是配额问题）；
//   - 冷却时长优先读响应 Retry-After 头（秒数形式，OpenAI/Anthropic/OpenRouter 均如此），
//     头缺失或不可解析时由熔断层取默认值（15s）。
//
// Error() 文案保持与原 "LLM API error: status=..." 同构，日志与监控不破相容。
type RateLimitError struct {
	StatusCode int
	RetryAfter time.Duration
	Body       string
}

var _ error = (*RateLimitError)(nil)

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("LLM API error: status=%d body=%s", e.StatusCode, e.Body)
}

func parseRetryAfterSeconds(header string) time.Duration {
	if header == "" {
		return 0
	}
	sec, err := strconv.Atoi(header)
	if err != nil || sec < 0 {
		return 0
	}
	return time.Duration(sec) * time.Second
}
