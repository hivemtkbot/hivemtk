package service

import (
	"context"
	"math"
	"testing"
	"time"

	"hivemtk-user/internal/model"
)

func TestDecayScore_HalfLife7Days(t *testing.T) {
	now := time.Now()
	last := now.Add(-168 * time.Hour)
	if got := DecayScore(1.0, last, now); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("168h 后得分应为 0.5，got %v", got)
	}
	if got := DecayScore(0.8, last, now); math.Abs(got-0.4) > 1e-9 {
		t.Errorf("168h 后 0.8 应衰减为 0.4，got %v", got)
	}

	half := now.Add(-84 * time.Hour)
	want := 1.0 / math.Sqrt2
	if got := DecayScore(1.0, half, now); math.Abs(got-want) > 1e-9 {
		t.Errorf("84h 后得分应为 %v，got %v", want, got)
	}
}

func TestDecayScore_ZeroElapsed(t *testing.T) {
	now := time.Now()
	if got := DecayScore(0.7, now, now); math.Abs(got-0.7) > 1e-12 {
		t.Errorf("零间隔不应衰减，got %v", got)
	}
}

func TestDecayScore_ClampAndFuture(t *testing.T) {
	now := time.Now()
	if got := DecayScore(-1, now, now); got != 0 {
		t.Errorf("负 confidence 应 clamp 为 0，got %v", got)
	}
	future := now.Add(time.Hour)
	if got := DecayScore(2.0, future, now); got != 1.0 {
		t.Errorf("confidence>1 且未来事件应得 1.0（clamp 后不衰减），got %v", got)
	}
}

func TestDecayScore_MonotonicInTime(t *testing.T) {
	now := time.Now()
	prev := DecayScore(1.0, now.Add(-24*time.Hour), now)
	for h := 48; h <= 240; h += 24 {
		cur := DecayScore(1.0, now.Add(-time.Duration(h)*time.Hour), now)
		if cur >= prev {
			t.Fatalf("得分应随时间单调递减：%dh=%v <= prev=%v", h, cur, prev)
		}
		prev = cur
	}
}

func TestDecayShouldArchiveMemory(t *testing.T) {
	cases := []struct {
		score, conf float64
		want        bool
	}{
		{0.149, 1.0, true},
		{0.15, 1.0, false},
		{0.151, 1.0, false},
		{1.0, 0.199, true},
		{1.0, 0.2, false},
	}
	for _, c := range cases {
		if got := shouldArchiveMemory(c.score, c.conf); got != c.want {
			t.Errorf("shouldArchiveMemory(%v,%v)=%v, want %v", c.score, c.conf, got, c.want)
		}
	}
}

func TestDecayItemConfidence(t *testing.T) {
	if got := itemDecayConfidence(model.MemoryItem{Importance: 5}); got != 0.5 {
		t.Errorf("无 metadata 时应取 importance/10，got %v", got)
	}
	if got := itemDecayConfidence(model.MemoryItem{Importance: 5, Metadata: model.JSONMap{"confidence": 0.9}}); got != 0.9 {
		t.Errorf("metadata.confidence 优先，got %v", got)
	}
	if got := itemDecayConfidence(model.MemoryItem{Importance: 5, Metadata: model.JSONMap{"confidence": 1.5}}); got != 1 {
		t.Errorf("confidence>1 应 clamp 为 1，got %v", got)
	}
	if got := itemDecayConfidence(model.MemoryItem{Importance: 5, Metadata: model.JSONMap{"confidence": 0}}); got != 0.5 {
		t.Errorf("confidence<=0 视为无效走 importance 兜底，got %v", got)
	}
}

func TestDecayRunMemoryDecayJobNilDB(t *testing.T) {
	stats, err := RunMemoryDecayJob(context.Background(), nil, false)
	if err != nil {
		t.Fatalf("nil db 不应报错，got %v", err)
	}
	if stats == nil || stats.Scanned != 0 || stats.Archived != 0 || stats.DryRun {
		t.Errorf("nil db 应返回零统计，got %+v", stats)
	}
}

func TestDecayRunMemoryDecayOnceNilDB(t *testing.T) {
	if got := RunMemoryDecayOnce(nil); got != 0 {
		t.Errorf("nil db 应返回 0，got %d", got)
	}
}

func TestDecayStartMemoryDecayLoopCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	StartMemoryDecayLoop(ctx, nil, 10*time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)
}
