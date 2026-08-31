package service

import (
	"context"
	"testing"
	"time"
)

func newRecallMonitor() *RagRecallMonitorService {
	return NewRagRecallMonitorService(nil, 0, 0)
}

// 1) 构造
func TestRagRecallMonitor_NewService(t *testing.T) {
	s := NewRagRecallMonitorService(nil, 10*time.Second, time.Hour)
	if s.interval != 10*time.Second {
		t.Errorf("Expected interval=10s, got %v", s.interval)
	}
	if s.window != time.Hour {
		t.Errorf("Expected window=1h, got %v", s.window)
	}

	s2 := newRecallMonitor()
	if s2.interval != RagRecallMonitorDefaultInterval {
		t.Errorf("Default interval mismatch")
	}
	if s2.window != RagRecallMonitorDefaultWindow {
		t.Errorf("Default window mismatch")
	}
}

// 2) Collect 空数据场景
func TestRagRecallMonitor_Collect_NilDB(t *testing.T) {
	s := newRecallMonitor()
	_, err := s.Collect(context.Background(), time.Now().Add(-time.Hour), time.Now())
	if err == nil {
		t.Error("Expected error for nil db")
	}
}

func TestRagRecallMonitor_Collect_EndBeforeStart(t *testing.T) {
	s := newRecallMonitor()
	now := time.Now()
	_, err := s.Collect(context.Background(), now, now.Add(-time.Hour))
	if err == nil {
		t.Error("Expected error for end<start")
	}
}

func TestRagRecallMonitor_CollectAndStore_NilDB(t *testing.T) {
	s := newRecallMonitor()
	_, err := s.CollectAndStore(context.Background(), time.Now().Add(-time.Hour), time.Now())
	if err == nil {
		t.Error("Expected error for nil db")
	}
	snap, _ := s.GetLatestSnapshot(context.Background())
	if snap == nil {
		_ = snap
	}
}

// 3) GetLatestSnapshot
func TestRagRecallMonitor_GetLatestSnapshot_Empty(t *testing.T) {
	s := newRecallMonitor()
	snap, ts := s.GetLatestSnapshot(context.Background())
	if snap != nil {
		t.Errorf("Expected nil snapshot, got %v", snap)
	}
	if !ts.IsZero() {
		t.Errorf("Expected zero time, got %v", ts)
	}
}

// 4) Start/Stop
func TestRagRecallMonitor_StartStop(t *testing.T) {
	s := NewRagRecallMonitorService(nil, 50*time.Millisecond, time.Hour)
	s.Start(context.Background())
	s.Start(context.Background())
	time.Sleep(80 * time.Millisecond)
	s.Stop(context.Background())
	s.Stop(context.Background())
}

// 5) EnsureSchema
func TestRagRecallMonitor_EnsureSchema_NilDB(t *testing.T) {
	s := newRecallMonitor()
	if err := s.EnsureSchema(context.Background()); err == nil {
		t.Error("Expected error for nil db")
	}
}

// 6) ListSnapshots
func TestRagRecallMonitor_ListSnapshots_NilDB(t *testing.T) {
	s := newRecallMonitor()
	_, err := s.ListSnapshots(context.Background(), 10)
	if err == nil {
		t.Error("Expected error for nil db")
	}
}

// 7) 越界参数 fallback
func TestRagRecallMonitor_BoundaryParams(t *testing.T) {
	s := NewRagRecallMonitorService(nil, -1, -1)
	if s.interval != RagRecallMonitorDefaultInterval {
		t.Errorf("Expected default interval, got %v", s.interval)
	}
	if s.window != RagRecallMonitorDefaultWindow {
		t.Errorf("Expected default window, got %v", s.window)
	}
}
