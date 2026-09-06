package bridge

import (
	"testing"
)

// TestIsIngestDuplicate_ReasonKeywords 验证上报 ack 闭环的原因判定：
// 命中幂等/拦截关键词的原因应被标记为 Duplicate（前端据此停发该 event_id），
// 正常原因不应误标。这是前端 uplink._markConfirmedFromResponse 正确闭环的服务端契约。
func TestIsIngestDuplicate_ReasonKeywords(t *testing.T) {
	positive := []string{
		"msg_id already exists",
		"intercepted by dedup middleware",
		"platform echo detected",
		"duplicate delivery in 5min",
		"skip due to cooldown",
		"already exists in db",
	}
	for _, r := range positive {
		if !isIngestDuplicate(r) {
			t.Errorf("应判定为重复: %q", r)
		}
	}

	negative := []string{
		"",
		"ai queued for processing",
		"accepted new message",
		"human locked",
		"queued for ai",
	}
	for _, r := range negative {
		if isIngestDuplicate(r) {
			t.Errorf("不应判定为重复: %q", r)
		}
	}
}
