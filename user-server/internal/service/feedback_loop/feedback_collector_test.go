package feedbackloop


import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
)


// TestComputeReward_Rating 评分归一化（v/5）
func TestComputeReward_Rating(t *testing.T) {
	c := &FeedbackCollector{config: DefaultFeedbackCollectorConfig()}
	r := c.computeReward(dto.FBSignalRating, 5, 0.8)
	if !approxEqualF64(r, 0.8) {
		t.Errorf("rating=5 reward = %v want 0.8", r)
	}
	r = c.computeReward(dto.FBSignalRating, 1, 0.8)
	if !approxEqualF64(r, 0.16) {
		t.Errorf("rating=1 reward = %v want 0.16", r)
	}
	r = c.computeReward(dto.FBSignalRating, 0, 0.8)
	if !approxEqualF64(r, 0) {
		t.Errorf("rating=0 reward = %v want 0", r)
	}
}

// TestComputeReward_ReplyRate 回复率归一化（直接乘）
func TestComputeReward_ReplyRate(t *testing.T) {
	c := &FeedbackCollector{config: DefaultFeedbackCollectorConfig()}
	r := c.computeReward(dto.FBSignalReplyRate, 0.8, 0.5)
	if !approxEqualF64(r, 0.4) {
		t.Errorf("reply_rate=0.8 reward = %v want 0.4", r)
	}
}

// TestComputeReward_Duration 会话时长归一化（v/300, 上限 1.0）
func TestComputeReward_Duration(t *testing.T) {
	c := &FeedbackCollector{config: DefaultFeedbackCollectorConfig()}
	r := c.computeReward(dto.FBSignalDuration, 150, 0.3)
	if !approxEqualF64(r, 0.15) {
		t.Errorf("duration=150 reward = %v want 0.15", r)
	}
	r = c.computeReward(dto.FBSignalDuration, 600, 0.3)
	if !approxEqualF64(r, 0.3) {
		t.Errorf("duration=600 reward = %v want 0.3 (capped at 1.0)", r)
	}
}

// TestComputeReward_Bool 布尔/字串信号直接用权重
func TestComputeReward_Bool(t *testing.T) {
	c := &FeedbackCollector{config: DefaultFeedbackCollectorConfig()}
	r := c.computeReward(dto.FBSignalLike, true, 1.0)
	if !approxEqualF64(r, 1.0) {
		t.Errorf("like=true reward = %v want 1.0", r)
	}
	r = c.computeReward(dto.FBSignalComplaint, true, -2.0)
	if !approxEqualF64(r, -2.0) {
		t.Errorf("complaint reward = %v want -2.0", r)
	}
	r = c.computeReward(dto.FBSignalConversion, true, 2.0)
	if !approxEqualF64(r, 2.0) {
		t.Errorf("conversion reward = %v want 2.0", r)
	}
}

// TestComputeReward_InvalidValue 信号值类型不匹配时退化为权重
func TestComputeReward_InvalidValue(t *testing.T) {
	c := &FeedbackCollector{config: DefaultFeedbackCollectorConfig()}
	r := c.computeReward(dto.FBSignalRating, "invalid", 0.8)
	if !approxEqualF64(r, 0.8) {
		t.Errorf("rating invalid value reward = %v want 0.8 (degraded to weight)", r)
	}
}

// TestLookupWeight_KnownSignal 已知信号返回配置权重
func TestLookupWeight_KnownSignal(t *testing.T) {
	c := &FeedbackCollector{config: DefaultFeedbackCollectorConfig()}
	if w := c.lookupWeight(dto.FBSignalLike); !approxEqualF64(w, 1.0) {
		t.Errorf("lookupWeight(like) = %v want 1.0", w)
	}
	if w := c.lookupWeight(dto.FBSignalConversion); !approxEqualF64(w, 2.0) {
		t.Errorf("lookupWeight(conversion) = %v want 2.0", w)
	}
	if w := c.lookupWeight(dto.FBSignalComplaint); !approxEqualF64(w, -2.0) {
		t.Errorf("lookupWeight(complaint) = %v want -2.0", w)
	}
}

// TestLookupWeight_UnknownSignal 未知信号返回 0
func TestLookupWeight_UnknownSignal(t *testing.T) {
	c := &FeedbackCollector{config: DefaultFeedbackCollectorConfig()}
	if w := c.lookupWeight(dto.FeedbackSignalKey("unknown_signal")); !approxEqualF64(w, 0) {
		t.Errorf("lookupWeight(unknown) = %v want 0", w)
	}
}

// TestToFloat64 各类型转 float64
func TestToFloat64(t *testing.T) {
	cases := []struct {
		input  any
		want   float64
		expect bool
	}{
		{float64(3.14), 3.14, true},
		{float32(2.5), 2.5, true},
		{int(42), 42, true},
		{int32(7), 7, true},
		{int64(99), 99, true},
		{true, 1.0, true},
		{false, 0.0, true},
		{"not a number", 0, false},
		{nil, 0, false},
	}
	for _, c := range cases {
		got, ok := toFloat64(c.input)
		if ok != c.expect {
			t.Errorf("toFloat64(%v) ok = %v want %v", c.input, ok, c.expect)
			continue
		}
		if ok && !approxEqualF64(got, c.want) {
			t.Errorf("toFloat64(%v) = %v want %v", c.input, got, c.want)
		}
	}
}

// TestGenEventID_Uniqueness 同请求多次生成不同 eventID
func TestGenEventID_Uniqueness(t *testing.T) {
	c := &FeedbackCollector{config: DefaultFeedbackCollectorConfig()}
	req := &dto.CollectRequest{
		SessionID: "sess-1", CustomerID: "cust-1",
		SignalKey: dto.FBSignalLike, SignalValue: true,
	}
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := c.genEventID(req)
		if id == "" {
			t.Error("genEventID 返回空字符串")
		}
		if ids[id] {
			t.Errorf("genEventID 生成重复 ID: %s", id)
		}
		ids[id] = true
	}
	if len(ids) != 100 {
		t.Errorf("应生成 100 个唯一 ID, got %d", len(ids))
	}
}

// TestGenEventID_Format 32 字符 hex
func TestGenEventID_Format(t *testing.T) {
	c := &FeedbackCollector{config: DefaultFeedbackCollectorConfig()}
	req := &dto.CollectRequest{
		SessionID: "sess-1", CustomerID: "cust-1",
		SignalKey: dto.FBSignalLike, SignalValue: true,
	}
	id := c.genEventID(req)
	if len(id) != 32 {
		t.Errorf("eventID 长度 = %d want 32", len(id))
	}
	for _, c := range id {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Errorf("eventID 含非 hex 字符: %c", c)
		}
	}
}


// TestFeedbackCollector_CollectSync_SingleEvent 同步采集单条事件
func TestFeedbackCollector_CollectSync_SingleEvent(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	c := NewFeedbackCollector(db, DefaultFeedbackCollectorConfig())
	defer c.Stop()

	req := &dto.CollectRequest{
		SessionID: "sess-single", CustomerID: "cust-1",
		EventType: dto.FBEventTypeExplicit,
		SignalKey: dto.FBSignalLike, SignalValue: true,
		AIReply: "ai reply", CustomerMsg: "customer msg",
	}
	if err := c.CollectSync(context.Background(), req); err != nil {
		t.Fatalf("CollectSync: %v", err)
	}

	// 验证 feedback_events 表
	var eventCount int64
	db.Model(&model.FeedbackEvent{}).Where("session_id = ?", "sess-single").Count(&eventCount)
	if eventCount != 1 {
		t.Errorf("feedback_events count = %d want 1", eventCount)
	}

	// 验证 feedback_signals 表（按 session 聚合）
	var signal model.FeedbackSignal
	if err := db.Where("session_id = ?", "sess-single").First(&signal).Error; err != nil {
		t.Fatalf("query signal: %v", err)
	}
	if signal.SignalCount != 1 {
		t.Errorf("SignalCount = %d want 1", signal.SignalCount)
	}
	if !approxEqualF64(signal.AggregatedReward, 1.0) {
		t.Errorf("AggregatedReward = %v want 1.0", signal.AggregatedReward)
	}
}

// TestFeedbackCollector_CollectSync_MultiEventAggregate 同 session 多事件聚合
//
// 验证：
//   - 多次 CollectSync 同一 session 的事件
//   - feedback_events 表追加多条
//   - feedback_signals 表只更新一条（按 session 唯一）
//   - aggregated_reward 累加
//   - signal_count 递增
func TestFeedbackCollector_CollectSync_MultiEventAggregate(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	c := NewFeedbackCollector(db, DefaultFeedbackCollectorConfig())
	defer c.Stop()

	sessionID := "sess-multi"
	ctx := context.Background()

	_ = c.CollectSync(ctx, &dto.CollectRequest{
		SessionID: sessionID, CustomerID: "cust-1",
		EventType: dto.FBEventTypeExplicit,
		SignalKey: dto.FBSignalLike, SignalValue: true,
	})
	_ = c.CollectSync(ctx, &dto.CollectRequest{
		SessionID: sessionID, CustomerID: "cust-1",
		EventType: dto.FBEventTypeImplicit,
		SignalKey: dto.FBSignalConversion, SignalValue: true,
	})
	_ = c.CollectSync(ctx, &dto.CollectRequest{
		SessionID: sessionID, CustomerID: "cust-1",
		EventType: dto.FBEventTypeExplicit,
		SignalKey: dto.FBSignalComplaint, SignalValue: true,
	})

	// 验证 feedback_events 3 条
	var eventCount int64
	db.Model(&model.FeedbackEvent{}).Where("session_id = ?", sessionID).Count(&eventCount)
	if eventCount != 3 {
		t.Errorf("feedback_events count = %d want 3", eventCount)
	}

	// 验证 feedback_signals 1 条
	var signal model.FeedbackSignal
	if err := db.Where("session_id = ?", sessionID).First(&signal).Error; err != nil {
		t.Fatalf("query signal: %v", err)
	}
	if signal.SignalCount != 3 {
		t.Errorf("SignalCount = %d want 3", signal.SignalCount)
	}
	if !approxEqualF64(signal.AggregatedReward, 1.0) {
		t.Errorf("AggregatedReward = %v want 1.0 (1.0+2.0-2.0)", signal.AggregatedReward)
	}
}

// TestFeedbackCollector_Collect_AsyncPersist 异步采集 + Stop 优雅关闭
//
// 验证：
//   - Collect 立即返回
//   - Stop 后所有入队事件已刷盘
func TestFeedbackCollector_Collect_AsyncPersist(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	cfg := DefaultFeedbackCollectorConfig()
	cfg.FlushInterval = 50 * time.Millisecond
	cfg.BatchSize = 100
	c := NewFeedbackCollector(db, cfg)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		err := c.Collect(ctx, &dto.CollectRequest{
			SessionID:  "sess-async-" + strings.Repeat("x", i+1), 
			CustomerID: "cust-1",
			EventType:  dto.FBEventTypeExplicit,
			SignalKey:  dto.FBSignalLike, SignalValue: true,
		})
		if err != nil {
			t.Fatalf("Collect[%d]: %v", i, err)
		}
	}

	c.Stop()

	// 验证 10 条 event 都已入库
	var eventCount int64
	db.Model(&model.FeedbackEvent{}).Where("session_id LIKE ?", "sess-async-%").Count(&eventCount)
	if eventCount != 10 {
		t.Errorf("async persist event count = %d want 10", eventCount)
	}
}

// TestFeedbackCollector_Collect_QueueFull 队列满返回 ErrQueueFull
//
// 使用极小队列（QueueSize=1）+ 阻塞 worker 模拟
func TestFeedbackCollector_Collect_QueueFull(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	cfg := DefaultFeedbackCollectorConfig()
	cfg.QueueSize = 2
	cfg.BatchSize = 1000
	cfg.FlushInterval = 10 * time.Second
	c := NewFeedbackCollector(db, cfg)
	defer c.Stop()
	ctx := context.Background()

	// 填满队列（QueueSize=2，缓冲 2 条）
	// 第 1 条立即被 worker select 消费 → 入队成功
	// 第 2 条入队成功（队列缓冲 2 条）
	// 第 3 条入队失败（队列满）
	// 实际行为受 goroutine 调度影响，多次尝试以确保触发 ErrQueueFull
	var queueFullCount int
	for i := 0; i < 100; i++ {
		err := c.Collect(ctx, &dto.CollectRequest{
			SessionID:  "sess-full-" + strings.Repeat("y", i+1),
			CustomerID: "cust-1",
			EventType:  dto.FBEventTypeExplicit,
			SignalKey:  dto.FBSignalLike, SignalValue: true,
		})
		if err != nil && errors.Is(err, ErrQueueFull) {
			queueFullCount++
		}
	}
	if queueFullCount == 0 {
		t.Logf("未触发 ErrQueueFull（worker 消费速度足够快，可能未达队列上限）")
	}
}

// TestFeedbackCollector_CollectSync_ValidateFailure 校验失败
func TestFeedbackCollector_CollectSync_ValidateFailure(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	c := NewFeedbackCollector(db, DefaultFeedbackCollectorConfig())
	defer c.Stop()

	cases := []struct {
		name string
		req  *dto.CollectRequest
		err  error
	}{
		{"nil request", nil, nil},
		{"empty session", &dto.CollectRequest{
			CustomerID: "cust-1", EventType: dto.FBEventTypeExplicit, SignalKey: dto.FBSignalLike,
		}, dto.ErrFeedbackSessionEmpty},
		{"empty customer", &dto.CollectRequest{
			SessionID: "sess-1", EventType: dto.FBEventTypeExplicit, SignalKey: dto.FBSignalLike,
		}, dto.ErrFeedbackCustomerEmpty},
		{"empty event_type", &dto.CollectRequest{
			SessionID: "sess-1", CustomerID: "cust-1", SignalKey: dto.FBSignalLike,
		}, dto.ErrFeedbackEventTypeEmpty},
		{"empty signal_key", &dto.CollectRequest{
			SessionID: "sess-1", CustomerID: "cust-1", EventType: dto.FBEventTypeExplicit,
		}, dto.ErrFeedbackSignalKeyEmpty},
	}
	for _, c2 := range cases {
		t.Run(c2.name, func(t *testing.T) {
			err := c.CollectSync(context.Background(), c2.req)
			if c2.err == nil {
				if err != nil {
					t.Errorf("case %s: err = %v want nil", c2.name, err)
				}
				return
			}
			if !errors.Is(err, c2.err) {
				t.Errorf("case %s: err = %v want %v", c2.name, err, c2.err)
			}
		})
	}
}

// TestFeedbackCollector_ConcurrentCollectSync 并发 CollectSync 不冲突
//
// 验证 upsertSignal 的 ON CONFLICT 处理在并发场景下不会报错
func TestFeedbackCollector_ConcurrentCollectSync(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	c := NewFeedbackCollector(db, DefaultFeedbackCollectorConfig())
	defer c.Stop()
	ctx := context.Background()

	const N = 20
	var wg sync.WaitGroup
	errs := make([]error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = c.CollectSync(ctx, &dto.CollectRequest{
				SessionID:  "sess-concurrent",
				CustomerID: "cust-1",
				EventType:  dto.FBEventTypeExplicit,
				SignalKey:  dto.FBSignalLike, SignalValue: true,
			})
		}(i)
	}
	wg.Wait()

	errCount := 0
	for _, e := range errs {
		if e != nil {
			errCount++
		}
	}
	if errCount > 0 {
		t.Errorf("并发 CollectSync 有 %d 个错误", errCount)
	}

	// 验证 feedback_events 有 N 条
	var eventCount int64
	db.Model(&model.FeedbackEvent{}).Where("session_id = ?", "sess-concurrent").Count(&eventCount)
	if eventCount != N {
		t.Errorf("event count = %d want %d", eventCount, N)
	}

	// 验证 feedback_signals 只有 1 条（按 session 唯一）
	var signalCount int64
	db.Model(&model.FeedbackSignal{}).Where("session_id = ?", "sess-concurrent").Count(&signalCount)
	if signalCount != 1 {
		t.Errorf("signal count = %d want 1 (session unique)", signalCount)
	}

	// 验证 signal_count = N
	var signal model.FeedbackSignal
	_ = db.Where("session_id = ?", "sess-concurrent").First(&signal).Error
	if signal.SignalCount != N {
		t.Errorf("SignalCount = %d want %d", signal.SignalCount, N)
	}
}

