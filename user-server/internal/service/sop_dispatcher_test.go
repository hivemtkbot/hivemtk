package service

// sop_dispatcher_test.go SOP 调度器与 Outbox/StuckDetector 集成测试（P0-1 §13.6 验收）
//
// 覆盖：
//  1. SOPRetryPolicy.Backoff 指数退避计算（纯单元测试）
//  2. SOPExecutionDispatcher Dispatch/DispatchOrLog/Stop（不需要 DB）
//  3. SOPOutboxDispatcher.processDueTimers timer 到期派发（需要 PG）
//  4. SOPOutboxDispatcher 多实例幂等性（需要 PG）
//  5. SOPStuckDetector.scanStuckExecutions 卡死检测（需要 PG）
//
// 需要 PG 的测试通过 testutil.NewTestDB 初始化，PG 不可用时自动 t.Skip。
// 私域独立部署：无 merchant_id 字段。

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/testutil"
)

// ===== SOPRetryPolicy 单元测试 =====

func TestSOPRetryPolicy_Backoff(t *testing.T) {
	p := DefaultSOPRetryPolicy()
	// attempt=1 应返回 InitialBackoff=1s
	if d := p.Backoff(context.Background(), 1); d != 1*time.Second {
		t.Errorf("Backoff(1)=%v want=1s", d)
	}
	// attempt=2 应返回 2s
	if d := p.Backoff(context.Background(), 2); d != 2*time.Second {
		t.Errorf("Backoff(2)=%v want=2s", d)
	}
	// attempt=3 应返回 4s
	if d := p.Backoff(context.Background(), 3); d != 4*time.Second {
		t.Errorf("Backoff(3)=%v want=4s", d)
	}
}

func TestSOPRetryPolicy_BackoffCappedAtMax(t *testing.T) {
	p := &SOPRetryPolicy{
		MaxAttempts:    5,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     5 * time.Second,
		Multiplier:     2.0,
	}
	// attempt=10 应被 cap 在 MaxBackoff=5s
	if d := p.Backoff(context.Background(), 10); d != 5*time.Second {
		t.Errorf("Backoff(10)=%v want=5s (capped)", d)
	}
}

func TestSOPRetryPolicy_BackoffAttemptZeroOrNegative(t *testing.T) {
	p := DefaultSOPRetryPolicy()
	// attempt<=1 应返回 InitialBackoff
	if d := p.Backoff(context.Background(), 0); d != 1*time.Second {
		t.Errorf("Backoff(0)=%v want=1s", d)
	}
	if d := p.Backoff(context.Background(), -1); d != 1*time.Second {
		t.Errorf("Backoff(-1)=%v want=1s", d)
	}
}

// ===== SOPExecutionDispatcher Dispatch/Stop 测试（不需要 DB）=====

func TestSOPExecutionDispatcher_DispatchQueued(t *testing.T) {
	// 测试任务能被放入队列
	d := &SOPExecutionDispatcher{
		dispatchQueue: make(chan *dispatchTask, 10),
		stopCh:        make(chan struct{}),
		retryPolicy:   DefaultSOPRetryPolicy(),
	}
	task := &dispatchTask{ExecutionID: 1, NodeID: "n1", Attempt: 0, TraceID: "t1"}
	if err := d.Dispatch(context.Background(), task); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	// 队列中应有 1 个任务
	if len(d.dispatchQueue) != 1 {
		t.Errorf("queue len=%d want=1", len(d.dispatchQueue))
	}
}

func TestSOPExecutionDispatcher_DispatchQueueFull(t *testing.T) {
	// 队列满时 Dispatch 应返回错误（背压）
	d := &SOPExecutionDispatcher{
		dispatchQueue: make(chan *dispatchTask, 1), // 容量 1
		stopCh:        make(chan struct{}),
		retryPolicy:   DefaultSOPRetryPolicy(),
	}
	d.dispatchQueue <- &dispatchTask{ExecutionID: 1, NodeID: "n1"}
	// 再 Dispatch 应失败
	err := d.Dispatch(context.Background(), &dispatchTask{ExecutionID: 2, NodeID: "n2"})
	if err == nil {
		t.Error("expected error when queue full")
	}
}

func TestSOPExecutionDispatcher_DispatchAfterStop(t *testing.T) {
	// 停止后 Dispatch 应返回错误
	d := &SOPExecutionDispatcher{
		dispatchQueue: make(chan *dispatchTask, 10),
		stopCh:        make(chan struct{}),
		retryPolicy:   DefaultSOPRetryPolicy(),
	}
	close(d.stopCh)
	err := d.Dispatch(context.Background(), &dispatchTask{ExecutionID: 1, NodeID: "n1"})
	if err == nil {
		t.Error("expected error after stop")
	}
}

func TestSOPExecutionDispatcher_DispatchOrLogNoPanic(t *testing.T) {
	// DispatchOrLog 失败时不应 panic，仅记录日志
	d := &SOPExecutionDispatcher{
		dispatchQueue: make(chan *dispatchTask, 0), // 容量 0，必然满
		stopCh:        make(chan struct{}),
		retryPolicy:   DefaultSOPRetryPolicy(),
	}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("DispatchOrLog panicked: %v", r)
		}
	}()
	d.DispatchOrLog(&dispatchTask{ExecutionID: 1, NodeID: "n1"})
}

// ===== SOPOutboxDispatcher / SOPStuckDetector 集成测试（需要 PG）=====

func TestSOPOutboxDispatcher_ProcessDueTimers_FiresPendingTimers(t *testing.T) {
	db := testutil.NewTestDB(t,
		&model.SOPExecution{},
		&model.SOPExecEvent{},
		&model.SOPTimer{},
		&model.SOPOutbox{},
	)

	// 创建一个已过期的 pending timer
	exec := &model.SOPExecution{
		SOPID:       1,
		CustomerID:  "cust_001",
		Status:      SOPStatusRunning,
		CurrentNode: "wait_node",
		StartedAt:   time.Now(),
	}
	if err := db.Create(exec).Error; err != nil {
		t.Fatalf("create execution: %v", err)
	}

	timer := &model.SOPTimer{
		ExecutionID: exec.ID,
		NodeID:      "wait_node",
		WaitEvent:   WaitEventTimer,
		WaitUntil:   time.Now().Add(-1 * time.Second), // 已过期
		Status:      "pending",
	}
	if err := db.Create(timer).Error; err != nil {
		t.Fatalf("create timer: %v", err)
	}

	// 用原子计数器记录派发任务数
	var dispatchedCount int32
	mockDispatcher := &mockExecDispatcher{
		dispatchFn: func(task *dispatchTask) {
			atomic.AddInt32(&dispatchedCount, 1)
			if task.ExecutionID != exec.ID {
				t.Errorf("task.ExecutionID=%d want=%d", task.ExecutionID, exec.ID)
			}
			if task.NodeID != "wait_node" {
				t.Errorf("task.NodeID=%s want=wait_node", task.NodeID)
			}
		},
	}

	outbox := NewSOPOutboxDispatcher(db, nil)
	outbox.execDispatcher = mockDispatcher
	outbox.processDueTimers(context.Background())

	if atomic.LoadInt32(&dispatchedCount) != 1 {
		t.Errorf("dispatchedCount=%d want=1", dispatchedCount)
	}

	// 验证 timer 已标记为 fired
	var updated model.SOPTimer
	db.First(&updated, timer.ID)
	if updated.Status != "fired" {
		t.Errorf("timer status=%s want=fired", updated.Status)
	}
	if updated.FiredAt == nil {
		t.Error("timer FiredAt should not be nil")
	}
}

func TestSOPOutboxDispatcher_ProcessDueTimers_SkipsFutureTimers(t *testing.T) {
	db := testutil.NewTestDB(t,
		&model.SOPExecution{},
		&model.SOPTimer{},
	)

	exec := &model.SOPExecution{
		SOPID: 1, CustomerID: "c1", Status: SOPStatusRunning,
		CurrentNode: "wait", StartedAt: time.Now(),
	}
	db.Create(exec)

	// 未来 1 小时才到期的 timer
	futureTimer := &model.SOPTimer{
		ExecutionID: exec.ID,
		NodeID:      "wait",
		WaitEvent:   WaitEventTimer,
		WaitUntil:   time.Now().Add(1 * time.Hour),
		Status:      "pending",
	}
	db.Create(futureTimer)

	var dispatchedCount int32
	mockDispatcher := &mockExecDispatcher{
		dispatchFn: func(task *dispatchTask) {
			atomic.AddInt32(&dispatchedCount, 1)
		},
	}
	outbox := NewSOPOutboxDispatcher(db, nil)
	outbox.execDispatcher = mockDispatcher
	outbox.processDueTimers(context.Background())

	if atomic.LoadInt32(&dispatchedCount) != 0 {
		t.Errorf("dispatchedCount=%d want=0 (future timer should not fire)", dispatchedCount)
	}

	// timer 应仍为 pending
	var updated model.SOPTimer
	db.First(&updated, futureTimer.ID)
	if updated.Status != "pending" {
		t.Errorf("future timer status=%s want=pending", updated.Status)
	}
}

func TestSOPOutboxDispatcher_ProcessDueTimers_MultiInstanceIdempotent(t *testing.T) {
	// 验收标准：多实例并发时同一 timer 只被派发一次
	db := testutil.NewTestDB(t,
		&model.SOPExecution{},
		&model.SOPTimer{},
	)

	exec := &model.SOPExecution{
		SOPID: 1, CustomerID: "c1", Status: SOPStatusRunning,
		CurrentNode: "wait", StartedAt: time.Now(),
	}
	db.Create(exec)

	timer := &model.SOPTimer{
		ExecutionID: exec.ID,
		NodeID:      "wait",
		WaitEvent:   WaitEventTimer,
		WaitUntil:   time.Now().Add(-1 * time.Second),
		Status:      "pending",
	}
	db.Create(timer)

	var dispatchedCount int32
	mockDispatcher := &mockExecDispatcher{
		dispatchFn: func(task *dispatchTask) {
			atomic.AddInt32(&dispatchedCount, 1)
		},
	}

	// 模拟两个 outbox 实例并发扫描
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o := NewSOPOutboxDispatcher(db, nil)
			o.execDispatcher = mockDispatcher
			o.processDueTimers(context.Background())
		}()
	}
	wg.Wait()

	if atomic.LoadInt32(&dispatchedCount) != 1 {
		t.Errorf("dispatchedCount=%d want=1 (multi-instance idempotent)", dispatchedCount)
	}
}

func TestSOPStuckDetector_ScanSkipsRunningExecution(t *testing.T) {
	// 正常 running 且最近有事件的 Execution 不应被恢复
	db := testutil.NewTestDB(t,
		&model.SOPExecution{},
		&model.SOPExecEvent{},
		&model.SOPTimer{},
	)

	now := time.Now()
	recentEvent := now.Add(-1 * time.Minute) // 1 分钟前有事件

	exec := &model.SOPExecution{
		SOPID: 1, CustomerID: "c1", Status: SOPStatusRunning,
		CurrentNode: "n1", StartedAt: now.Add(-1 * time.Hour),
		LastEventAt: &recentEvent,
	}
	db.Create(exec)

	var dispatchedCount int32
	mockDispatcher := &mockExecDispatcher{
		dispatchFn: func(task *dispatchTask) {
			atomic.AddInt32(&dispatchedCount, 1)
		},
	}

	detector := NewSOPStuckDetector(db, nil)
	detector.execDispatcher = mockDispatcher
	detector.SetMaxIdleTime(context.Background(), 30 * time.Minute)
	detector.scanStuckExecutions(context.Background())

	if atomic.LoadInt32(&dispatchedCount) != 0 {
		t.Errorf("dispatchedCount=%d want=0 (recent event, not stuck)", dispatchedCount)
	}
}

func TestSOPStuckDetector_ScanSkipsWaitNodeWithPendingTimer(t *testing.T) {
	// 有 pending timer 的 wait 节点不算卡死
	db := testutil.NewTestDB(t,
		&model.SOPExecution{},
		&model.SOPExecEvent{},
		&model.SOPTimer{},
	)

	now := time.Now()
	oldEvent := now.Add(-2 * time.Hour) // 2 小时前事件

	exec := &model.SOPExecution{
		SOPID: 1, CustomerID: "c1", Status: SOPStatusRunning,
		CurrentNode: "wait", StartedAt: now.Add(-48 * time.Hour), // 超过 24h
		LastEventAt: &oldEvent,
		WaitEvent:   WaitEventCustomerReply,
	}
	db.Create(exec)

	// 创建 pending timer
	timer := &model.SOPTimer{
		ExecutionID: exec.ID,
		NodeID:      "wait",
		WaitEvent:   WaitEventCustomerReply,
		WaitUntil:   now.Add(20 * time.Hour), // 还没到期
		Status:      "pending",
	}
	db.Create(timer)

	var dispatchedCount int32
	mockDispatcher := &mockExecDispatcher{
		dispatchFn: func(task *dispatchTask) {
			atomic.AddInt32(&dispatchedCount, 1)
		},
	}

	detector := NewSOPStuckDetector(db, nil)
	detector.execDispatcher = mockDispatcher
	detector.SetMaxIdleTime(context.Background(), 30 * time.Minute)
	detector.scanStuckExecutions(context.Background())

	if atomic.LoadInt32(&dispatchedCount) != 0 {
		t.Errorf("dispatchedCount=%d want=0 (wait node with pending timer not stuck)", dispatchedCount)
	}
}

func TestSOPStuckDetector_ScanRecoversTrulyStuckExecution(t *testing.T) {
	// 真正卡死的 Execution（无 pending timer + 无近期事件 + 超 24h）应被恢复
	db := testutil.NewTestDB(t,
		&model.SOPExecution{},
		&model.SOPExecEvent{},
		&model.SOPTimer{},
	)

	now := time.Now()
	oldEvent := now.Add(-2 * time.Hour)

	exec := &model.SOPExecution{
		SOPID: 1, CustomerID: "c1", Status: SOPStatusRunning,
		CurrentNode: "stuck_node", StartedAt: now.Add(-48 * time.Hour),
		LastEventAt: &oldEvent,
	}
	db.Create(exec)
	// 无 pending timer、无近期 sop_exec_events

	var dispatchedCount int32
	var dispatchedNodeID string
	var dispatchedExecID uint
	mockDispatcher := &mockExecDispatcher{
		dispatchFn: func(task *dispatchTask) {
			atomic.AddInt32(&dispatchedCount, 1)
			dispatchedNodeID = task.NodeID
			dispatchedExecID = task.ExecutionID
		},
	}

	detector := NewSOPStuckDetector(db, nil)
	detector.execDispatcher = mockDispatcher
	detector.SetMaxIdleTime(context.Background(), 30 * time.Minute)
	detector.scanStuckExecutions(context.Background())

	if atomic.LoadInt32(&dispatchedCount) != 1 {
		t.Fatalf("dispatchedCount=%d want=1 (stuck execution should be recovered)", dispatchedCount)
	}
	if dispatchedNodeID != "stuck_node" {
		t.Errorf("dispatched NodeID=%s want=stuck_node", dispatchedNodeID)
	}
	if dispatchedExecID != exec.ID {
		t.Errorf("dispatched ExecutionID=%d want=%d", dispatchedExecID, exec.ID)
	}
}

// ===== mockExecDispatcher 用于测试，模拟 SOPExecutionDispatcher 的 DispatchOrLog =====

type mockExecDispatcher struct {
	dispatchFn func(task *dispatchTask)
}

func (m *mockExecDispatcher) DispatchOrLog(task *dispatchTask) {
	if m.dispatchFn != nil {
		m.dispatchFn(task)
	}
}

// 引用 context 包以避免 unused import（在测试函数未用到时）
var _ = context.Background
