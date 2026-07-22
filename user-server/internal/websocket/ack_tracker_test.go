package websocket

import (
	"testing"
)

// TestPendingAck_Track 记录待 ACK
func TestPendingAck_Track(t *testing.T) {
	p := NewPendingAck()
	p.Track("s1", 100)
	p.Track("s1", 101)
	p.Track("s1", 102)

	pending := p.Pending("s1")
	if len(pending) != 3 {
		t.Errorf("pending count = %d, want 3", len(pending))
	}
}

// TestPendingAck_Ack 客户端确认后清理
func TestPendingAck_Ack(t *testing.T) {
	p := NewPendingAck()
	p.Track("s1", 100)
	p.Track("s1", 101)
	p.Track("s1", 102)

	// 批量 ack
	count := p.Ack("s1", 100, 101)
	if count != 2 {
		t.Errorf("acked = %d, want 2", count)
	}

	pending := p.Pending("s1")
	if len(pending) != 1 || pending[0] != 102 {
		t.Errorf("remaining = %v, want [102]", pending)
	}
}

// TestPendingAck_AckUnknown 不存在的 seq 不报错
func TestPendingAck_AckUnknown(t *testing.T) {
	p := NewPendingAck()
	p.Track("s1", 100)

	if count := p.Ack("s1", 999); count != 0 {
		t.Errorf("acked unknown = %d, want 0", count)
	}
	if pending := p.Pending("s1"); len(pending) != 1 {
		t.Errorf("pending count = %d, want 1", len(pending))
	}
}

// TestPendingAck_PendingSince 拉取 seq > sinceSeq 的未 ACK
func TestPendingAck_PendingSince(t *testing.T) {
	p := NewPendingAck()
	p.Track("s1", 100)
	p.Track("s1", 101)
	p.Track("s1", 105)
	p.Track("s1", 110)

	pending := p.PendingSince("s1", 102)
	if len(pending) != 2 {
		t.Fatalf("since=102 pending = %d, want 2 (105 + 110)", len(pending))
	}
	for _, s := range pending {
		if s <= 102 {
			t.Errorf("seq %d should be > 102", s)
		}
	}
}

// TestPendingAck_Drop 客户端断开清理全部
func TestPendingAck_Drop(t *testing.T) {
	p := NewPendingAck()
	p.Track("s1", 100)
	p.Track("s1", 101)
	p.Track("s2", 200)

	p.Drop("s1")

	if pending := p.Pending("s1"); len(pending) != 0 {
		t.Errorf("s1 not dropped: %v", pending)
	}
	if pending := p.Pending("s2"); len(pending) != 1 {
		t.Errorf("s2 should remain: %v", pending)
	}
}

// TestPendingAck_EmptySession 空 session 返回 nil
func TestPendingAck_EmptySession(t *testing.T) {
	p := NewPendingAck()
	if pending := p.Pending("nonexistent"); pending != nil {
		t.Errorf("expected nil for unknown session, got %v", pending)
	}
	if pending := p.PendingSince("nonexistent", 0); pending != nil {
		t.Errorf("expected nil for unknown session since, got %v", pending)
	}
}

// TestPendingAck_TrackInvalid 输入合法性
func TestPendingAck_TrackInvalid(t *testing.T) {
	p := NewPendingAck()
	// 空 sessionID / seq=0 都应被忽略
	p.Track("", 100)
	p.Track("s1", 0)

	if pending := p.Pending(""); len(pending) != 0 {
		t.Errorf("empty sessionID should be ignored: %v", pending)
	}
	if pending := p.Pending("s1"); len(pending) != 0 {
		t.Errorf("seq=0 should be ignored: %v", pending)
	}
}

// TestPendingAck_NilSafe nil 接收器不 panic
func TestPendingAck_NilSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("nil receiver panicked: %v", r)
		}
	}()
	var p *PendingAck
	p.Track("s1", 100)
	p.Ack("s1", 100)
	p.Pending("s1")
	p.PendingSince("s1", 0)
	p.Drop("s1")
}

// TestGlobalPendingAck_Singleton 全局单例可获取
func TestGlobalPendingAck_Singleton(t *testing.T) {
	a := GlobalPendingAck()
	b := GlobalPendingAck()
	if a != b {
		t.Error("GlobalPendingAck should return same instance")
	}
	// Track + Ack 验证连通
	a.Track("singleton_test", 99999)
	if a.Ack("singleton_test", 99999) != 1 {
		t.Error("singleton Track/Ack failed")
	}
}
