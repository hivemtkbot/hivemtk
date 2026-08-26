package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// ============================================================================
// M4：sop_timers 列下沉回归测试
// expires_at/max_wait_at/claim_count 原先仅存于 Payload JSONB，无法建索引、
// 无法用 SQL 条件扫描。下沉实体列后：写入落列、扫描走列查询（部分索引），
// 旧数据（仅 payload 有值）兼容读取。
// ============================================================================

// TestSOPTimer_SinkColumns_Persist AutoMigrate 注册确认：三列可持久化、可查询
func TestSOPTimer_SinkColumns_Persist(t *testing.T) {
	db := testutil.NewTestDB(t, &model.SOPTimer{})

	now := time.Now().Add(-time.Minute)
	expires := now.Add(time.Hour)
	timer := &model.SOPTimer{
		ExecutionID: 1,
		NodeID:      "wait_1",
		WaitEvent:   WaitEventTimer,
		WaitUntil:   expires,
		Status:      "pending",
		ExpiresAt:   &expires,
		MaxWaitAt:   &now,
		ClaimCount:  2,
	}
	if err := db.Create(timer).Error; err != nil {
		t.Fatalf("create timer with sink columns: %v", err)
	}

	var got model.SOPTimer
	if err := db.First(&got, timer.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.MaxWaitAt == nil || !got.MaxWaitAt.Equal(now) {
		t.Errorf("MaxWaitAt = %v, want %v", got.MaxWaitAt, now)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, now.Add(time.Hour))
	}
	if got.ClaimCount != 2 {
		t.Errorf("ClaimCount = %d, want 2", got.ClaimCount)
	}
}

// TestSOPDispatcher_SweepSkipsOverdueByColumn 列查询扫描 max_wait_at 过期 → skipped
func TestSOPDispatcher_SweepSkipsOverdueByColumn(t *testing.T) {
	db := testutil.NewTestDB(t, &model.SOPExecution{}, &model.SOPExecEvent{}, &model.SOPTimer{})
	exec := &model.SOPExecution{SOPID: 1, CustomerID: "c1", Status: SOPStatusRunning, CurrentNode: "wait", StartedAt: time.Now()}
	db.Create(exec)

	past := time.Now().Add(-time.Hour)
	timer := &model.SOPTimer{
		ExecutionID: exec.ID,
		NodeID:      "wait",
		WaitEvent:   WaitEventCustomerReply,
		WaitUntil:   time.Now().Add(24 * time.Hour),
		Status:      "pending",
		MaxWaitAt:   &past,
	}
	db.Create(timer)

	var dispatched int32
	outbox := NewSOPOutboxDispatcher(db, nil)
	outbox.execDispatcher = &mockExecDispatcher{dispatchFn: func(task *dispatchTask) {
		if task.SkipWait {
			atomic.AddInt32(&dispatched, 1)
		}
	}}
	outbox.sweepPendingTimers(context.Background(), time.Now())

	var updated model.SOPTimer
	db.First(&updated, timer.ID)
	if updated.Status != sopTimerStatusSkipped {
		t.Errorf("status=%s want=skipped（列值过期应被扫描到）", updated.Status)
	}
	if atomic.LoadInt32(&dispatched) != 1 {
		t.Errorf("SkipWait dispatched=%d want=1", dispatched)
	}
}

// TestSOPDispatcher_SweepLegacyPayloadOnlyTimer 旧数据兼容：仅 payload 有 max_wait_at（列 NULL）仍能被跳过
func TestSOPDispatcher_SweepLegacyPayloadOnlyTimer(t *testing.T) {
	db := testutil.NewTestDB(t, &model.SOPExecution{}, &model.SOPTimer{})
	exec := &model.SOPExecution{SOPID: 1, CustomerID: "c1", Status: SOPStatusRunning, StartedAt: time.Now()}
	db.Create(exec)

	past := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	timer := &model.SOPTimer{
		ExecutionID: exec.ID,
		NodeID:      "wait",
		WaitEvent:   WaitEventCustomerReply,
		WaitUntil:   time.Now().Add(24 * time.Hour),
		Status:      "pending",
		Payload:     model.JSONMap{"max_wait_at": past}, // 无实体列（历史行）
	}
	db.Create(timer)

	outbox := NewSOPOutboxDispatcher(db, nil)
	outbox.execDispatcher = &mockExecDispatcher{dispatchFn: func(task *dispatchTask) {}}
	outbox.sweepPendingTimers(context.Background(), time.Now())

	var updated model.SOPTimer
	db.First(&updated, timer.ID)
	if updated.Status != sopTimerStatusSkipped {
		t.Errorf("status=%s want=skipped（旧 payload 数据应回退读取）", updated.Status)
	}
}

// TestSOPDispatcher_ClaimCountColumnDeadLetter claim_count 实体列 ≥5 → dead_letter
func TestSOPDispatcher_ClaimCountColumnDeadLetter(t *testing.T) {
	db := testutil.NewTestDB(t, &model.SOPTimer{})
	timer := &model.SOPTimer{
		ExecutionID: 9,
		NodeID:      "wait",
		WaitEvent:   WaitEventCustomerReply,
		WaitUntil:   time.Now().Add(24 * time.Hour),
		Status:      "pending",
		ClaimCount:  sopTimerMaxClaims,
	}
	db.Create(timer)

	outbox := NewSOPOutboxDispatcher(db, nil)
	outbox.sweepPendingTimers(context.Background(), time.Now())

	var updated model.SOPTimer
	db.First(&updated, timer.ID)
	if updated.Status != sopTimerStatusDeadLetter {
		t.Errorf("status=%s want=dead_letter（claim_count 列应驱动死信迁移）", updated.Status)
	}
	if updated.FiredAt == nil {
		t.Error("dead_letter timer FiredAt should be set")
	}
}

// TestSOPDispatcher_BumpClaimWritesColumn 认领失败 → 实体列与 payload 双写递增
func TestSOPDispatcher_BumpClaimWritesColumn(t *testing.T) {
	db := testutil.NewTestDB(t, &model.SOPTimer{})
	timer := &model.SOPTimer{
		ExecutionID: 9,
		NodeID:      "wait",
		WaitEvent:   WaitEventCustomerReply,
		WaitUntil:   time.Now().Add(24 * time.Hour),
		Status:      "pending",
		ClaimCount:  1,
		Payload:     model.JSONMap{"claim_count": 1},
	}
	db.Create(timer)

	outbox := NewSOPOutboxDispatcher(db, nil)
	outbox.bumpTimerClaimOrDeadLetter(context.Background(), timer, time.Now())

	var updated model.SOPTimer
	db.First(&updated, timer.ID)
	if updated.ClaimCount != 2 {
		t.Errorf("ClaimCount=%d want=2", updated.ClaimCount)
	}
	if got := timerClaimCount(&updated); got != 2 {
		t.Errorf("timerClaimCount=%d want=2", got)
	}
}
