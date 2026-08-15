package bridge

import (
	"testing"
	"time"
)

func TestHTTPReplyBuffer_Basic(t *testing.T) {
	b := newHTTPReplyBuffer()

	t.Run("Push + Pull 匹配", func(t *testing.T) {
		reply := &UnifiedReply{Channel: "xiaohongshu", AccountID: "xhs_1", ConversationID: "conv_1", Content: "你好", ReplyToEventID: "evt_1"}
		b.Push(reply)

		got := b.Pull("xiaohongshu", "conv_1", "evt_1")
		if got == nil {
			t.Fatal("Pull 返回 nil")
		}
		if got.Content != "你好" {
			t.Errorf("Content = %q, want 你好", got.Content)
		}
		if again := b.Pull("xiaohongshu", "conv_1", "evt_1"); again != nil {
			t.Errorf("二次 Pull 应返回 nil，实际 %+v", again)
		}
	})

	t.Run("conversation_id 不匹配 → 放回", func(t *testing.T) {
		b := newHTTPReplyBuffer()
		reply := &UnifiedReply{Channel: "xiaohongshu", AccountID: "xhs_1", ConversationID: "conv_1", Content: "你好"}
		b.Push(reply)

		if got := b.Pull("xiaohongshu", "conv_2", ""); got != nil {
			t.Errorf("不匹配的 conv_id 应返回 nil，实际 %+v", got)
		}
		if got := b.Pull("xiaohongshu", "conv_1", ""); got == nil {
			t.Error("不匹配后放回：再次匹配应能拉到")
		}
	})

	t.Run("空 conversation_id 匹配任意", func(t *testing.T) {
		b := newHTTPReplyBuffer()
		b.Push(&UnifiedReply{Channel: "douyin", AccountID: "dy_1", ConversationID: "conv_X", Content: "hi"})

		got := b.Pull("douyin", "", "")
		if got == nil {
			t.Error("空 conv_id 应匹配任意")
		}
	})

	t.Run("空 buffer 返回 nil", func(t *testing.T) {
		b := newHTTPReplyBuffer()
		if got := b.Pull("empty_channel", "", ""); got != nil {
			t.Errorf("空 buffer 应返回 nil，实际 %+v", got)
		}
	})

	t.Run("FIFO 容量上限", func(t *testing.T) {
		b := newHTTPReplyBuffer()
		for i := 0; i < b.maxLen+10; i++ {
			b.Push(&UnifiedReply{Channel: "test_cap", AccountID: "a", ConversationID: "c", Content: "msg"})
		}
		count := 0
		for {
			r := b.Pull("test_cap", "", "")
			if r == nil {
				break
			}
			count++
			if count > b.maxLen*2 {
				t.Fatal("Pull 无限循环")
			}
		}
		if count > b.maxLen {
			t.Errorf("Pull 总数 = %d, 应 <= maxLen = %d", count, b.maxLen)
		}
	})

	t.Run("waitForReply 超时返回 nil", func(t *testing.T) {
		b := newHTTPReplyBuffer()
		start := time.Now()
		got := b.waitForReply("empty_ch", "", "", 300*time.Millisecond)
		elapsed := time.Since(start)
		if got != nil {
			t.Errorf("空 buffer waitForReply 应返回 nil，实际 %+v", got)
		}
		if elapsed < 300*time.Millisecond {
			t.Errorf("waitForReply 提前返回：elapsed=%v", elapsed)
		}
		if elapsed > 600*time.Millisecond {
			t.Errorf("waitForReply 超时过长：elapsed=%v", elapsed)
		}
	})

	t.Run("waitForReply 命中立即返回", func(t *testing.T) {
		b := newHTTPReplyBuffer()
		b.Push(&UnifiedReply{Channel: "fast", AccountID: "a", ConversationID: "c", Content: "hi"})

		start := time.Now()
		got := b.waitForReply("fast", "c", "", 5*time.Second)
		elapsed := time.Since(start)
		if got == nil {
			t.Fatal("应命中")
		}
		if elapsed > 500*time.Millisecond {
			t.Errorf("命中耗时过长：elapsed=%v", elapsed)
		}
	})
}

