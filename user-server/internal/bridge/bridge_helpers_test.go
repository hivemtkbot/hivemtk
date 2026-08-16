package bridge

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"hivemtk-user/internal/model"
)

// TestMaskTokenBridge 验证 token 脱敏格式（保留前 4 位 + 总长度 + 尾部 chars）与扩展端对齐
func TestMaskTokenBridge(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"空 token", "", ""},
		{"短 token (1 char)", "x", "x***(1 chars)"},
		{"短 token (4 chars)", "abcd", "abcd***(4 chars)"},
		{"普通 JWT (>= 5 chars)", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c", "eyJh***(108 chars)"},
		{"中文/特殊字符 token（Go len 按字节计）", "中文token", "中文to***(11 chars)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := maskTokenBridge(tc.input)
			if got != tc.want {
				t.Errorf("maskTokenBridge(%q) = %q, want %q", tc.input, got, tc.want)
			}
			if len(tc.input) > 4 && strings.Contains(got, tc.input) {
				t.Errorf("脱敏结果包含完整 token，泄漏风险: input=%q got=%q", tc.input, got)
			}
		})
	}
}

// TestItoa 验证简单整数转字符串（避免引入 strconv 依赖膨胀）
func TestItoa(t *testing.T) {
	cases := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{9, "9"},
		{10, "10"},
		{91, "91"},
		{12345, "12345"},
		{-1, "-1"},
		{-123, "-123"},
	}
	for _, tc := range cases {
		got := itoa(tc.input)
		if got != tc.want {
			t.Errorf("itoa(%d) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestDescribeUpstreamQuery 验证 query 解析与 token 字段脱敏行为
func TestDescribeUpstreamQuery(t *testing.T) {
	t.Run("空 query", func(t *testing.T) {
		v := url.Values{}
		got := describeUpstreamQuery(v)
		if len(got) != 0 {
			t.Errorf("空 query 应输出空 map，实际 %v", got)
		}
	})

	t.Run("普通参数不解密", func(t *testing.T) {
		v := url.Values{}
		v.Set("channel", "douyin")
		v.Set("account_id", "12345")
		v.Set("conversation_id", "abc")
		got := describeUpstreamQuery(v)
		if got["channel"] != "douyin" {
			t.Errorf("channel = %q, want %q", got["channel"], "douyin")
		}
		if got["account_id"] != "12345" {
			t.Errorf("account_id = %q, want %q", got["account_id"], "12345")
		}
		if got["conversation_id"] != "abc" {
			t.Errorf("conversation_id = %q, want %q", got["conversation_id"], "abc")
		}
	})

	t.Run("token 字段必须脱敏", func(t *testing.T) {
		v := url.Values{}
		v.Set("channel", "douyin")
		v.Set("token", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c")
		got := describeUpstreamQuery(v)
		if !strings.HasPrefix(got["token"], "eyJh") {
			t.Errorf("token 脱敏应保留前 4 位，实际 %q", got["token"])
		}
		if strings.Contains(got["token"], "SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c") {
			t.Errorf("token 脱敏结果泄漏完整签名段: %q", got["token"])
		}
		if !strings.HasSuffix(got["token"], "chars)") {
			t.Errorf("token 脱敏应以 'chars)' 结尾，实际 %q", got["token"])
		}
	})

	t.Run("空 token 字段", func(t *testing.T) {
		v := url.Values{}
		v.Set("token", "")
		got := describeUpstreamQuery(v)
		if got["token"] != "" {
			t.Errorf("空 token 应输出空串，实际 %q", got["token"])
		}
	})

	t.Run("真实桥接 query 场景", func(t *testing.T) {
		v := url.Values{}
		v.Set("channel", "xiaohongshu")
		v.Set("account_id", "xhs_abc_123")
		v.Set("conversation_id", "conv_xyz")
		v.Set("token", "eyJhbGciOiJIUzI1NiJ9.payload.sig")
		got := describeUpstreamQuery(v)
		if got["channel"] != "xiaohongshu" {
			t.Errorf("channel 错: %q", got["channel"])
		}
		if got["account_id"] != "xhs_abc_123" {
			t.Errorf("account_id 错: %q", got["account_id"])
		}
		if got["conversation_id"] != "conv_xyz" {
			t.Errorf("conversation_id 错: %q", got["conversation_id"])
		}
		if !strings.HasPrefix(got["token"], "eyJh") {
			t.Errorf("token 应保留前 4 位，实际 %q", got["token"])
		}
		if strings.Contains(got["token"], "payload") || strings.Contains(got["token"], "sig") {
			t.Errorf("token 脱敏泄漏 payload/sig 段: %q", got["token"])
		}
	})
}

// TestMaskTokenBridge_LogFormatAlignment 与扩展端 bridge-client.js describeUpstreamParams 的 mask 格式对齐：
//
//	格式：${v.slice(0, 4)}***(${v.length} chars)
//
// 因此 Go 侧输出也必须满足 "<4位前缀>***(<数字> chars)" 格式。
func TestMaskTokenBridge_LogFormatAlignment(t *testing.T) {
	token := "abcdefghij1234567890"
	got := maskTokenBridge(token)
	if !strings.HasPrefix(got, "abcd") {
		t.Errorf("应保留前 4 位 abcd，实际 %q", got)
	}
	if !strings.HasSuffix(got, "chars)") {
		t.Errorf("应以 'chars)' 结尾，实际 %q", got)
	}
	if !strings.Contains(got, "***(") {
		t.Errorf("应包含 '***(' 段，实际 %q", got)
	}
	idx := strings.Index(got, "***(")
	if idx < 0 {
		t.Fatalf("格式错误: %q", got)
	}
	tail := got[idx+4:] 
	tail = strings.TrimSuffix(tail, " chars)")
	var n int
	for _, c := range tail {
		if c < '0' || c > '9' {
			t.Fatalf("数字段含非数字字符: %q (raw=%q)", tail, got)
		}
		n = n*10 + int(c-'0')
	}
	if n != len(token) {
		t.Errorf("长度不匹配: got=%d want=%d (raw=%q)", n, len(token), got)
	}
}

// TestHistoryItemToEvent 验证「一个会话 = 一条消息、内含多轮历史」的帧→MessageEvent 映射：
//   - 会话元数据（channel/account/conversation/群）取自帧顶层 message
//   - 轮次字段（event_id/sender/content/direction）取自 item
//   - 群聊元数据从 item 回退到顶层 message（或Default）
//   - 出站轮次的 receiver_id = 会话对方（统一收信中心聚合键）
func TestHistoryItemToEvent(t *testing.T) {
	frameMsg := &UnifiedMessage{
		Channel:        "xiaohongshu",
		AccountID:      "acc1",
		ConversationID: "group-1",
		SenderType:     "customer",
		IsGroup:        true,
		GroupID:        "group-1",
		GroupName:      "产品交流群",
	}

	cases := []struct {
		name string
		item *HistoryItem
		want func(*testing.T, *model.MessageEvent)
	}{
		{
			name: "群聊客户轮：sender/群名取 item，覆盖顶层缺省",
			item: &HistoryItem{
				EventID:    "g1",
				SenderType: "customer",
				SenderID:   "group-1",
				SenderName: "张三",
				MsgType:    "text",
				Content:    "有人在吗",
				Timestamp:  1700000000000,
				Direction:  "inbound",
				IsGroup:    true,
				GroupID:    "group-1",
				GroupName:  "产品交流群",
			},
			want: func(t *testing.T, ev *model.MessageEvent) {
				if ev.EventID != "g1" || ev.Content != "有人在吗" || ev.SenderName != "张三" {
					t.Fatalf("轮次字段映射错误: %+v", ev)
				}
				if ev.SessionID != "xiaohongshu:acc1:group-1" {
					t.Fatalf("SessionID 错误: %s", ev.SessionID)
				}
				if !ev.IsGroup || ev.GroupID != "group-1" {
					t.Fatalf("群元数据错误: %+v", ev)
				}
				if ev.Extra["is_group"] != true || ev.Extra["group_id"] != "group-1" || ev.Extra["group_name"] != "产品交流群" {
					t.Fatalf("群 Extra 错误: %+v", ev.Extra)
				}
			},
		},
		{
			name: "1:1 出站轮：receiver_id = 会话 id（聚合到对方）",
			item: &HistoryItem{
				EventID:    "o1",
				SenderType: "agent",
				SenderID:   "acc1",
				MsgType:    "text",
				Content:    "您好，请问有什么可以帮您？",
				Timestamp:  1700000001000,
				Direction:  "outbound",
			},
			want: func(t *testing.T, ev *model.MessageEvent) {
				if ev.ReceiverID != "group-1" {
					t.Fatalf("出站 receiver_id 应为会话 id，实际 %q", ev.ReceiverID)
				}
			},
		},
		{
			name: "item 缺省字段回退顶层 message（direction 空 → inbound，群字段继承）",
			item: &HistoryItem{
				EventID:   "i2",
				MsgType:   "text",
				Content:   "缺省方向",
				Timestamp: 0, 
			},
			want: func(t *testing.T, ev *model.MessageEvent) {
				if ev.MsgType != "text" || ev.Content != "缺省方向" {
					t.Fatalf("字段映射错误: %+v", ev)
				}
				if ev.GroupID != "group-1" {
					t.Fatalf("群 id 应继承顶层: %q", ev.GroupID)
				}
				if ev.Extra["group_name"] != "产品交流群" {
					t.Fatalf("群名应继承顶层: %+v", ev.Extra)
				}
				if ev.Timestamp.IsZero() {
					t.Fatal("timestamp 应为当前时间而非零值")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := historyItemToEvent(frameMsg, tc.item)
			tc.want(t, ev)
		})
	}
}

// TestHistoryItemToEvent_ToChannelConversion 验证渠道常量保持 web 变体（事件落库用桥接渠道）
func TestHistoryItemToEvent_ToChannelConversion(t *testing.T) {
	ev := historyItemToEvent(
		&UnifiedMessage{Channel: "douyin", AccountID: "a", ConversationID: "c"},
		&HistoryItem{EventID: "x", Content: "hi", Timestamp: time.Now().UnixMilli(), Direction: "inbound"},
	)
	if ev.Channel != "douyin" {
		t.Fatalf("渠道应保持 web 变体: %q", ev.Channel)
	}
}



