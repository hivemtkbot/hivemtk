package websocket

import (
	"context"
	"sync"
	"time"

	"hivemtk-user/internal/cache"
)


// PendingAck 待 ACK 跟踪表
// key: 会话 ID（visitor/agent 都用）
// value: seq -> 首次发送时间
type PendingAck struct {
	mu    sync.RWMutex
	items map[string]map[uint64]time.Time 
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
	// D15: 写穿 Redis（异步）
	if snapshot, ok := p.items[sessionID]; ok {
		pendingRedis.asyncSetJSON(sessionID, snapshot)
	}
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
	// D15: 写穿 Redis（异步）
	pendingRedis.asyncSetJSON(sessionID, pending)
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
	merged := make(map[uint64]time.Time, len(p.items[sessionID]))
	for s, ts := range p.items[sessionID] {
		merged[s] = ts
	}
	p.mu.RUnlock()
	// D15: 合并远端 Redis 快照（他副本/重启前写入）
	for s, ts := range pendingRedis.loadRemote(sessionID) {
		if _, exists := merged[s]; !exists {
			merged[s] = ts
		}
	}
	out := make([]uint64, 0, len(merged))
	for s := range merged {
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


// pendingRedisBacked D15：pending 写穿 Redis（单键 SetJSON map，接口无 Hash——审核修正 3）。
// 同一 session 同一时刻仅活跃于一个副本，跨副本并发写竞态后果=丢一条 pending（少补发），可接受。
// Track/Ack 热路径 fire-and-forget（goroutine），不阻塞。
type pendingRedisBacked struct {
	enabled func() bool
	backend func() cache.Cache
}

var pendingRedis = &pendingRedisBacked{
	enabled: func() bool { return seqIsRedis() && !redisDegraded.Load() },
	backend: func() cache.Cache { return seqBackend() },
}

func pendingKey(sessionID string) string { return "mtk:ws:pending:" + sessionID }

// asyncSetJSON fire-and-forget 写穿
func (p *pendingRedisBacked) asyncSetJSON(sessionID string, snapshot map[uint64]time.Time) {
	if !p.enabled() {
		return
	}
	c := p.backend()
	if c == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = c.SetJSON(ctx, pendingKey(sessionID), snapshot, 24*time.Hour)
	}()
}

// loadRemote 读远端 pending 快照（PendingSince 合并用）
func (p *pendingRedisBacked) loadRemote(sessionID string) map[uint64]time.Time {
	if !p.enabled() {
		return nil
	}
	c := p.backend()
	if c == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var remote map[uint64]time.Time
	if err := c.GetJSON(ctx, pendingKey(sessionID), &remote); err != nil {
		return nil
	}
	return remote
}
