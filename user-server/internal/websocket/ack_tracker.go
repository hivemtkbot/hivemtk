package websocket

import (
	"sync"
	"time"
)

// ============================================================================
// 投递 ACK 跟踪（ack_tracker.go）
// ----------------------------------------------------------------------------
// 设计动机：
//   - 客户端每收到一条消息后发 `{"type":"ack","seq":N}` 告知服务端
//   - 服务端清理 ack 队列，停止"该 seq 的离线补发"
//   - 未 ack 的消息保留在 delivered_at=NULL 等待下次重连
//
// 与已有 delivered_at 标记的关系：
//   - onConnect 拉取后批量标记 delivered_at（粗粒度，避免每条 UPDATE）
//   - ack 仅用于实时清点"客户端已确认"，不写库
//   - 保留"未 ack 但已 delivered_at"状态作为兜底
// ============================================================================

// PendingAck 待 ACK 跟踪表
// key: 会话 ID（visitor/agent 都用）
// value: seq -> 首次发送时间
type PendingAck struct {
	mu    sync.RWMutex
	items map[string]map[uint64]time.Time // sessionID -> seq -> firstSeen
}

// NewPendingAck 创建 ACK 跟踪表
func NewPendingAck() *PendingAck {
	return &PendingAck{items: make(map[string]map[uint64]time.Time)}
}

// Track 记录一条待 ACK 消息
func (p *PendingAck) Track(sessionID string, seq uint64) {
	if p == nil || sessionID == "" || seq == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.items[sessionID]; !ok {
		p.items[sessionID] = make(map[uint64]time.Time)
	}
	p.items[sessionID][seq] = time.Now()
}

// Ack 客户端确认收到 seq，可传多个（批量 ACK）
//
// 返回被清理的 seq 数量。
func (p *PendingAck) Ack(sessionID string, seqs ...uint64) int {
	if p == nil || sessionID == "" {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	pending, ok := p.items[sessionID]
	if !ok {
		return 0
	}
	count := 0
	for _, s := range seqs {
		if _, exists := pending[s]; exists {
			delete(pending, s)
			count++
		}
	}
	if len(pending) == 0 {
		delete(p.items, sessionID)
	}
	return count
}

// Pending 列出该 sessionID 下所有未 ACK 的 seq（按 firstSeen 升序）
func (p *PendingAck) Pending(sessionID string) []uint64 {
	if p == nil {
		return nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	pending, ok := p.items[sessionID]
	if !ok {
		return nil
	}
	out := make([]uint64, 0, len(pending))
	for s := range pending {
		out = append(out, s)
	}
	return out
}

// PendingSince 拉取 seq > sinceSeq 的所有未 ACK（用于重连后查缺补漏）
func (p *PendingAck) PendingSince(sessionID string, sinceSeq uint64) []uint64 {
	if p == nil {
		return nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	pending, ok := p.items[sessionID]
	if !ok {
		return nil
	}
	out := make([]uint64, 0, len(pending))
	for s := range pending {
		if s > sinceSeq {
			out = append(out, s)
		}
	}
	return out
}

// Drop 客户端断开时清空该 sessionID 的所有 pending（避免内存泄漏）
func (p *PendingAck) Drop(sessionID string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.items, sessionID)
}

// globalAckTracker 全局 ACK 跟踪器
var globalAckTracker = NewPendingAck()

// GlobalPendingAck 获取全局 ACK 跟踪器（供 visitor_handler.go 使用）
func GlobalPendingAck() *PendingAck {
	return globalAckTracker
}
