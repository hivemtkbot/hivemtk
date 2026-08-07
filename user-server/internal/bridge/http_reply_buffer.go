package bridge

import (
	"sync"
	"time"
)

// httpReplyBuffer HTTP 模式下的 AI 回复缓冲（遗留）
//
// 2026-08-05 架构重构（用户诉求）：bridge 改用 HTTP 长轮询后，
// AI 回复需要有个 transport-agnostic 的"暂存"通道供长轮询拉取。
// 流程：WebhookService → bridge.SendXxx → deliverWS → hub.Deliver (WS 投递) → Push 到 buffer
//
// 现状（2026-08-06 三通道架构后）：
//   - 生产 AI 回复已改为落库 message_hub(status=pending)，由扩展端 GET /api/bridge/outbox 拉取，
//     .NET 不再经此 buffer（HTTPReplyPullerRegistry / waitForAIReply 已移除）。
//   - 本 buffer 现仅由遗留接线 ReachAdapter.SendXxx → deliverHTTP 写入，已无读取方，
//     属死代码；保留以兼容 RegisterBridgeOutbound / Send* 接口测试。
//
// 容量策略：单 channel 最多保留 256 条 reply，超出时按 FIFO 淘汰。
// 内存占用：每条 reply 约 200B-2KB（content 截断 4KB），256 条 × 5 渠道 ≈ 2.5MB 上限。
type httpReplyBuffer struct {
	mu     sync.Mutex
	queues map[string]chan *UnifiedReply // key: channel
	maxLen int
}

// newHTTPReplyBuffer 构造 reply 缓冲；每渠道一个带缓冲 channel，buffer 满时 Push 阻塞 → Hub.Deliver 也阻塞 → 限速生效。
func newHTTPReplyBuffer() *httpReplyBuffer {
	return &httpReplyBuffer{
		queues: make(map[string]chan *UnifiedReply),
		maxLen: 256,
	}
}

// queue 按 channel 懒加载获取 queue
func (b *httpReplyBuffer) queue(channel string) chan *UnifiedReply {
	b.mu.Lock()
	defer b.mu.Unlock()
	q, ok := b.queues[channel]
	if !ok {
		q = make(chan *UnifiedReply, b.maxLen)
		b.queues[channel] = q
	}
	return q
}

// Push 推入一条 reply；FIFO 满时丢弃最早一条并 warn 日志。
func (b *httpReplyBuffer) Push(r *UnifiedReply) {
	if r == nil {
		return
	}
	q := b.queue(r.Channel)
	select {
	case q <- r:
	default:
		// 缓冲满：丢弃最早一条
		select {
		case <-q:
		default:
		}
		// 重新尝试一次（理论仅丢 1 条）
		select {
		case q <- r:
		default:
			// 真满（极端）：丢当前
		}
	}
}

// Pull 拉取一条匹配的 reply
// 匹配规则：
//   - conversationID 非空：必须匹配（空时匹配任意）
//   - replyToEventID 非空：必须匹配 ReplyToEventID（空时匹配任意）
// 拉取后从 buffer 移除（非阻塞）
// 未匹配时返回 nil
//
// 实现：先全部 drain 到切片，扫描匹配后剩余放回。
// 注意：放回时若 channel 已满（不可能，但防御性）会丢，但 maxLen=256 远超单次长轮询产生的 reply。
func (b *httpReplyBuffer) Pull(channel, conversationID, replyToEventID string) *UnifiedReply {
	if channel == "" {
		return nil
	}
	q := b.queue(channel)
	// 非持锁扫描：把所有元素 drain 到切片
	items := drainNonBlocking(q)
	if len(items) == 0 {
		return nil
	}
	var matched *UnifiedReply
	remaining := make([]*UnifiedReply, 0, len(items))
	for _, r := range items {
		if matched == nil {
			if (conversationID == "" || r.ConversationID == conversationID) &&
				(replyToEventID == "" || r.ReplyToEventID == replyToEventID) {
				matched = r
				continue
			}
		}
		remaining = append(remaining, r)
	}
	// 把不匹配的全部放回（顺序保持）
	for _, r := range remaining {
		select {
		case q <- r:
		default:
			// 缓冲真满（极端）：丢弃
		}
	}
	return matched
}

// drainNonBlocking 非阻塞 drain channel 内全部元素。
// 优化：直接 len(q) 知道 channel 内当前元素数，一次性 make。
func drainNonBlocking(q chan *UnifiedReply) []*UnifiedReply {
	out := make([]*UnifiedReply, 0, len(q))
	for {
		select {
		case r := <-q:
			out = append(out, r)
		default:
			return out
		}
	}
}

// waitForReply 带超时的非阻塞拉取（遗留：原由已移除的 waitForAIReply 调用）
func (b *httpReplyBuffer) waitForReply(channel, conversationID, replyToEventID string, timeout time.Duration) *UnifiedReply {
	deadline := time.Now().Add(timeout)
	pollInterval := 200 * time.Millisecond
	for {
		if r := b.Pull(channel, conversationID, replyToEventID); r != nil {
			return r
		}
		if time.Now().After(deadline) {
			return nil
		}
		time.Sleep(pollInterval)
	}
}
