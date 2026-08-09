package bridge

import (
	"testing"
)

// TestIsIngestDuplicate_ReasonKeywords 验证上报 ack 闭环的原因判定：
// 命中幂等/拦截关键词的原因应被标记为 Duplicate（前端据此停发该 event_id），
// 正常原因不应误标。这是前端 uplink._markConfirmedFromResponse 正确闭环的服务端契约。
func TestIsIngestDuplicate_ReasonKeywords(t *testing.T) {
	positive := []string{
		"msg_id already exists",          // 钩子2：msg_id 已落库
		"intercepted by dedup middleware", // 统一收件中间件拦截（回环/短时重复）
		"platform echo detected",          // 自/他回显
		"duplicate delivery in 5min",      // 重复投递
		"skip due to cooldown",            // 跳过
		"already exists in db",            // 兜底已存在
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
