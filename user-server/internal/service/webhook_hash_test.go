package service

import "testing"

// TestContentHashMsgIDCrossLanguageContract 守护「跨语言哈希契约（最高优先级）」：
// Go 端 ContentHashMsgID 必须与前端 types.js::contentHash 逐字节一致，否则所有
// 桥接回环去重 / AI 自回显拦截 / 消息幂等去重都会对中文内容静默失效。
//
// 契约：FNV-1a 32 位，输入 = channel + "|" + trim(content)（UTF-8 字节），
// 输出 = "mh:" + 8 位小写 hex。conversationID 严禁进入哈希输入。
func TestContentHashMsgIDCrossLanguageContract(t *testing.T) {
	const anchor = "mh:00550fed" // 前端 types.js 同款断言向量：contentHash('douyin','c1','你好')

	// 1) 与前端锚点逐字节一致（中文内容，UTF-8 编码口径）。
	if got := ContentHashMsgID("douyin", "c1", "你好"); got != anchor {
		t.Fatalf("cross-language anchor mismatch: ContentHashMsgID('douyin','c1','你好') = %q, want %q", got, anchor)
	}

	// 2) conversationID 必须被忽略：不同 conversationID 必须得到同一指纹（否则跨会话同内容指纹不一致 → 去重失效）。
	if got := ContentHashMsgID("douyin", "TOTALLY_DIFFERENT_CONV", "你好"); got != anchor {
		t.Fatalf("conversationID leaked into hash: got %q want %q (conv must be ignored)", got, anchor)
	}

	// 3) 输入须 TrimSpace：前后空白不应改变指纹。
	if got := ContentHashMsgID("douyin", "c1", "  你好  "); got != anchor {
		t.Fatalf("TrimSpace not applied: got %q want %q", got, anchor)
	}

	// 4) 内容不同 → 指纹必须不同。
	if got := ContentHashMsgID("douyin", "c1", "你好吗"); got == anchor {
		t.Fatalf("different content produced identical hash %q (collision)", got)
	}

	// 5) 同一输入确定性稳定（多次调用一致）。
	a := ContentHashMsgID("douyin", "c1", "你好")
	b := ContentHashMsgID("douyin", "c1", "你好")
	if a != b {
		t.Fatalf("hash not deterministic: %q vs %q", a, b)
	}

	// 6) 混合中英文 + emoji 的 UTF-8 多字节内容也能稳定产出非空指纹（覆盖常见私信内容）。
	mixed := ContentHashMsgID("xiaohongshu", "conv9", "在吗？我们家的面霜限时8折🥰")
	if len(mixed) != 11 || mixed[:3] != "mh:" {
		t.Fatalf("mixed UTF-8 content produced malformed hash: %q", mixed)
	}
	if mixed == anchor {
		t.Fatalf("mixed content accidentally collided with anchor %q", anchor)
	}
}

// TestContentHashWithSenderDistinguishesSender 守护「客户复述 AI 原话」与「AI 自回显」的区分：
// 同内容不同发送者必须产出不同指纹。
func TestContentHashWithSenderDistinguishesSender(t *testing.T) {
	base := ContentHashWithSender("douyin", "customer_123", "你好")
	self := ContentHashWithSender("douyin", "account_abc", "你好")
	if base == self {
		t.Fatalf("sender must affect hash: customer vs self produced identical %q", base)
	}
	// 与无 sender 的 MsgID 也应不同。
	if base == ContentHashMsgID("douyin", "c1", "你好") {
		t.Fatalf("ContentHashWithSender must differ from ContentHashMsgID on same content")
	}
	// 确定性稳定。
	if ContentHashWithSender("douyin", "account_abc", "你好") != self {
		t.Fatalf("ContentHashWithSender not deterministic")
	}
}
