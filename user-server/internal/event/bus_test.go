package event

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestEventBus_PublishSubscribe 基础发布订阅
func TestEventBus_PublishSubscribe(t *testing.T) {
	bus := New(1, 16)
	defer bus.Stop()

	var received atomic.Int32
	bus.Subscribe("test.topic", func(evt Event) error {
		if evt.Topic != "test.topic" {
			t.Errorf("expected topic test.topic, got %s", evt.Topic)
		}
		received.Add(1)
		return nil
	})

	bus.Publish(Event{Topic: "test.topic", Payload: "hello"})

	waitForCondition(t, func() bool { return received.Load() == 1 }, 100*time.Millisecond)
	if received.Load() != 1 {
		t.Errorf("expected 1 event received, got %d", received.Load())
	}
}

// TestEventBus_MultipleSubscribers 多订阅者：单事件分发到所有订阅者
func TestEventBus_MultipleSubscribers(t *testing.T) {
	bus := New(1, 16)
	defer bus.Stop()

	var count atomic.Int32
	bus.Subscribe("topic.a", func(evt Event) error {
		count.Add(1)
		return nil
	})
	bus.Subscribe("topic.a", func(evt Event) error {
		count.Add(1)
		return nil
	})

	bus.Publish(Event{Topic: "topic.a"})

	waitForCondition(t, func() bool { return count.Load() == 2 }, 100*time.Millisecond)
	if count.Load() != 2 {
		t.Errorf("expected 2 calls, got %d", count.Load())
	}
}

// TestEventBus_TopicIsolation 主题隔离：只有匹配的订阅者收到
func TestEventBus_TopicIsolation(t *testing.T) {
	bus := New(2, 16)
	defer bus.Stop()

	var aCount, bCount atomic.Int32
	bus.Subscribe("topic.a", func(evt Event) error {
		aCount.Add(1)
		return nil
	})
	bus.Subscribe("topic.b", func(evt Event) error {
		bCount.Add(1)
		return nil
	})

	bus.Publish(Event{Topic: "topic.a"})
	bus.Publish(Event{Topic: "topic.b"})

	waitForCondition(t, func() bool { return aCount.Load() == 1 && bCount.Load() == 1 }, 200*time.Millisecond)
	if aCount.Load() != 1 {
		t.Errorf("expected topic.a count 1, got %d", aCount.Load())
	}
	if bCount.Load() != 1 {
		t.Errorf("expected topic.b count 1, got %d", bCount.Load())
	}
}

// TestEventBus_HandlerError 不阻断：单个 handler 失败不影响后续 handler
func TestEventBus_HandlerError(t *testing.T) {
	bus := New(1, 16)
	defer bus.Stop()

	var executed atomic.Int32
	bus.Subscribe("topic.err", func(evt Event) error {
		return ErrTestHandlerFailure
	})
	bus.Subscribe("topic.err", func(evt Event) error {
		executed.Add(1)
		return nil
	})

	bus.Publish(Event{Topic: "topic.err"})

	waitForCondition(t, func() bool { return executed.Load() == 1 }, 100*time.Millisecond)
	if executed.Load() != 1 {
		t.Errorf("expected second handler to execute despite first failure, got %d", executed.Load())
	}
}

// TestEventBus_QueueFull 队列满丢弃：不阻塞调用方
func TestEventBus_QueueFull(t *testing.T) {
	// 队列容量 2，1 个 worker 阻塞消费
	bus := New(1, 2)
	defer bus.Stop()

	var processed atomic.Int32
	workerStarted := make(chan struct{})
	blockCh := make(chan struct{})
	bus.Subscribe("topic.full", func(evt Event) error {
		// 第一次调用时通知测试：worker 已开始处理
		select {
		case workerStarted <- struct{}{}:
		default:
		}
		<-blockCh // 阻塞直到测试释放
		processed.Add(1)
		return nil
	})

	// 发布第 1 个事件，worker 拿走后阻塞
	bus.Publish(Event{Topic: "topic.full"})

	// 等待 worker 确认已开始处理（此时队列为空）
	<-workerStarted

	// 再发布 3 个事件：2 个进队列，1 个被丢弃（队列容量 2）
	for i := 0; i < 3; i++ {
		bus.Publish(Event{Topic: "topic.full"})
	}

	// 释放阻塞，让 worker 处理剩余事件
	close(blockCh)

	// 最终处理数应为 3（第 1 个 + 队列中 2 个），第 4 个被丢弃
	waitForCondition(t, func() bool { return processed.Load() == 3 }, 500*time.Millisecond)
	if processed.Load() != 3 {
		t.Errorf("expected 3 processed (1 initial + 2 queued, 1 dropped), got %d", processed.Load())
	}
}

// TestEventBus_StopGraceful 优雅关闭：等待队列中事件处理完成
func TestEventBus_StopGraceful(t *testing.T) {
	bus := New(1, 64)

	var processed atomic.Int32
	bus.Subscribe("topic.stop", func(evt Event) error {
		processed.Add(1)
		return nil
	})

	// 发布 10 个事件
	for i := 0; i < 10; i++ {
		bus.Publish(Event{Topic: "topic.stop"})
	}

	// Stop 应等待所有事件处理完成
	bus.Stop()

	if processed.Load() != 10 {
		t.Errorf("expected all 10 events processed after Stop, got %d", processed.Load())
	}
}

// TestEventBus_StopIdempotent Stop 幂等：可重复调用
func TestEventBus_StopIdempotent(t *testing.T) {
	bus := New(1, 16)
	bus.Stop()
	bus.Stop() // 不应 panic
	bus.Stop() // 不应 panic
}

// TestEventBus_NilSafe nil 安全：nil 总线方法不 panic
func TestEventBus_NilSafe(t *testing.T) {
	var nilBus *EventBus
	nilBus.Publish(Event{Topic: "nil"}) // 不应 panic
	nilBus.Subscribe("nil", func(evt Event) error { return nil })
	nilBus.Stop()
	if nilBus.HasSubscribers("nil") {
		t.Error("nil bus should not have subscribers")
	}
}

// TestEventBus_HasSubscribers 检查订阅者
func TestEventBus_HasSubscribers(t *testing.T) {
	bus := New(1, 16)
	defer bus.Stop()

	if bus.HasSubscribers("topic.x") {
		t.Error("expected no subscribers initially")
	}

	bus.Subscribe("topic.x", func(evt Event) error { return nil })
	if !bus.HasSubscribers("topic.x") {
		t.Error("expected subscribers after Subscribe")
	}
}

// TestEventBus_AutoTimestamp 自动填充时间戳
func TestEventBus_AutoTimestamp(t *testing.T) {
	bus := New(1, 16)
	defer bus.Stop()

	var ts time.Time
	var wg sync.WaitGroup
	wg.Add(1)
	bus.Subscribe("topic.ts", func(evt Event) error {
		ts = evt.Timestamp
		wg.Done()
		return nil
	})

	before := time.Now()
	bus.Publish(Event{Topic: "topic.ts", Timestamp: time.Time{}}) // 零值
	wg.Wait()

	if ts.IsZero() {
		t.Error("expected timestamp to be auto-filled")
	}
	if ts.Before(before) {
		t.Error("timestamp should not be before publish time")
	}
}

// TestEventBus_ConcurrentPublish 并发发布
func TestEventBus_ConcurrentPublish(t *testing.T) {
	bus := New(4, 1024)
	defer bus.Stop()

	var received atomic.Int32
	bus.Subscribe("topic.concurrent", func(evt Event) error {
		received.Add(1)
		return nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Publish(Event{Topic: "topic.concurrent"})
		}()
	}
	wg.Wait()

	waitForCondition(t, func() bool { return received.Load() == 100 }, 500*time.Millisecond)
	if received.Load() != 100 {
		t.Errorf("expected 100 events, got %d", received.Load())
	}
}

// TestGlobalBus 全局单例
func TestGlobalBus(t *testing.T) {
	// 初始状态可能为 nil 或其他测试残留，先清理
	SetGlobalBus(nil)

	if GetGlobalBus() != nil {
		t.Error("expected nil global bus after SetGlobalBus(nil)")
	}

	// Publish 在全局总线为 nil 时应为 no-op
	Publish("topic.nil", "payload") // 不应 panic

	bus := New(1, 16)
	SetGlobalBus(bus)
	defer func() {
		SetGlobalBus(nil)
	}()

	if GetGlobalBus() != bus {
		t.Error("expected global bus to be set")
	}

	var received atomic.Int32
	var wg sync.WaitGroup
	wg.Add(1)
	bus.Subscribe("topic.global", func(evt Event) error {
		received.Add(1)
		wg.Done()
		return nil
	})

	// 使用全局 Publish 函数
	Publish("topic.global", "test")
	wg.Wait()

	if received.Load() != 1 {
		t.Errorf("expected 1 event via global Publish, got %d", received.Load())
	}

	// StopGlobal 应停止全局总线
	StopGlobal()
	if GetGlobalBus() != nil {
		t.Error("expected nil global bus after StopGlobal")
	}
}

// === 辅助 ===

var ErrTestHandlerFailure = newTestError("handler failure")

type testError string

func (e testError) Error() string { return string(e) }

func newTestError(s string) error { return testError(s) }

// waitForCondition 等待条件满足或超时
func waitForCondition(t *testing.T, cond func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
}
