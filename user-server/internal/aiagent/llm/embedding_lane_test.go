package llm

import (
	"context"
	"testing"
	"time"
)

// N-1 决策源：docs/architecture/MASTER_COMPETITIVE_DECISIONS.md M3 表 N-1
// 双 channel：online(cap=并发-1) 与 batch(cap=1)；batch 任务让路在线检索。

func TestNewEmbeddingLanesFrom_Capacities(t *testing.T) {
	cases := []struct {
		n          int
		wantOnline int
		wantBatch  int
	}{
		{n: 1, wantOnline: 1, wantBatch: 1}, // 保底：online 永不为 0
		{n: 2, wantOnline: 1, wantBatch: 1},
		{n: 4, wantOnline: 3, wantBatch: 1},
		{n: 0, wantOnline: 1, wantBatch: 1}, // 非法输入回退保底
	}
	for _, c := range cases {
		lanes := newEmbeddingLanesFrom(c.n)
		if got := cap(lanes.online); got != c.wantOnline {
			t.Errorf("n=%d online cap = %d, want %d", c.n, got, c.wantOnline)
		}
		if got := cap(lanes.batch); got != c.wantBatch {
			t.Errorf("n=%d batch cap = %d, want %d", c.n, got, c.wantBatch)
		}
	}
}

func TestEmbeddingLanes_Isolation_OnlineNotStarvedByBatch(t *testing.T) {
	lanes := newEmbeddingLanesFrom(2)

	// batch 车道被占满（cap=1，一个批量任务在飞）
	if err := acquireEmbeddingSlot(context.Background(), lanes.batch); err != nil {
		t.Fatalf("acquire batch: %v", err)
	}

	// online 车道不受 batch 占用影响，仍可获取槽位
	onlineCh := lanes.laneChannel(EmbeddingLaneOnline)
	done := make(chan error, 1)
	go func() { done <- acquireEmbeddingSlot(context.Background(), onlineCh) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("online acquire failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("online acquire blocked while only batch lane was busy: N-1 隔离失效")
	}

	// batch 车道 cap=1：批量槽位被占期间，后续 batch 请求必须阻塞
	batchCh := lanes.laneChannel(EmbeddingLaneBatch)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := acquireEmbeddingSlot(ctx, batchCh); err == nil {
		t.Fatal("batch slot acquired beyond cap=1")
	}

	releaseEmbeddingSlot(batchCh)
	done2 := make(chan error, 1)
	go func() { done2 <- acquireEmbeddingSlot(context.Background(), batchCh) }()
	select {
	case err := <-done2:
		if err != nil {
			t.Fatalf("batch acquire failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("batch acquire blocked unexpectedly")
	}
}

func TestEmbeddingLanes_OnlineCapRespected(t *testing.T) {
	lanes := newEmbeddingLanesFrom(4) // online cap=3

	for i := 0; i < 3; i++ {
		if err := acquireEmbeddingSlot(context.Background(), lanes.online); err != nil {
			t.Fatalf("acquire #%d: %v", i, err)
		}
	}

	// 第 4 个 online 请求必须阻塞（超出容量）
	blocked := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	blocked <- acquireEmbeddingSlot(ctx, lanes.online)
	if err := <-blocked; err == nil {
		t.Fatal("online slot acquired beyond capacity: 信号量未生效")
	}

	releaseEmbeddingSlot(lanes.online)
	if err := acquireEmbeddingSlot(context.Background(), lanes.online); err != nil {
		t.Fatalf("re-acquire after release: %v", err)
	}
}

func TestEmbeddingLanes_LaneChannelDefaultOnline(t *testing.T) {
	lanes := newEmbeddingLanesFrom(2)
	if lanes.laneChannel(EmbeddingLaneOnline) != lanes.online {
		t.Fatal("default lane must be online")
	}
	if lanes.laneChannel(EmbeddingLane(99)) != lanes.online {
		t.Fatal("unknown lane must fall back to online (API 兼容默认在线)")
	}
	if lanes.laneChannel(EmbeddingLaneBatch) != lanes.batch {
		t.Fatal("batch lane mismatch")
	}
}
