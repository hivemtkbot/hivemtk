package cron

import (
	"context"
	"errors"
	"testing"
	"time"
)

func testQuotaCron(fn func(ctx context.Context) (int64, error)) *WeComQuotaResetCron {
	c := NewWeComQuotaResetCron(nil)
	c.executeFn = fn
	return c
}

// TestNextDailyTick 当日时刻未到：返回今日
func TestNextDailyTick_BeforeToday(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 8, 26, 0, 3, 0, 0, loc)
	got := nextDailyTick(now, 0, 5, loc)
	want := time.Date(2026, 8, 26, 0, 5, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("next = %v, want %v", got, want)
	}
}

// TestNextDailyTick_AfterToday：返回明日
func TestNextDailyTick_AfterToday(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 8, 26, 0, 6, 0, 0, loc)
	got := nextDailyTick(now, 0, 5, loc)
	want := time.Date(2026, 8, 27, 0, 5, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("next = %v, want %v", got, want)
	}
}

// TestWeComQuotaResetCron_RunOnce_Delegates 调度执行应委托 ResetAllDailyQuotas 并透传结果
func TestWeComQuotaResetCron_RunOnce_Delegates(t *testing.T) {
	calls := 0
	c := testQuotaCron(func(ctx context.Context) (int64, error) {
		calls++
		return 7, nil
	})
	n, err := c.runOnce(context.Background())
	if err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if calls != 1 || n != 7 {
		t.Errorf("calls=%d n=%d, want calls=1 n=7", calls, n)
	}
}

// TestWeComQuotaResetCron_RunOnce_Error 执行错误应向上传递（由 loop 记录日志）
func TestWeComQuotaResetCron_RunOnce_Error(t *testing.T) {
	c := testQuotaCron(func(ctx context.Context) (int64, error) {
		return 0, errors.New("db down")
	})
	if _, err := c.runOnce(context.Background()); err == nil {
		t.Error("expected error to propagate")
	}
}

// TestWeComQuotaResetCron_RunOnce_PanicRecovered panic 应被隔离为错误
func TestWeComQuotaResetCron_RunOnce_PanicRecovered(t *testing.T) {
	c := testQuotaCron(func(ctx context.Context) (int64, error) {
		panic("boom")
	})
	if _, err := c.runOnce(context.Background()); err == nil {
		t.Error("expected panic converted to error")
	}
}

// TestWeComQuotaResetCron_StartStopIdempotent 幂等启动/停止，Stop 能及时退出调度协程
func TestWeComQuotaResetCron_StartStopIdempotent(t *testing.T) {
	c := testQuotaCron(func(ctx context.Context) (int64, error) { return 0, nil })
	done := make(chan struct{})
	go func() {
		c.Start(context.Background())
		c.Start(context.Background()) // 幂等
		c.Stop(context.Background())
		c.Stop(context.Background()) // 幂等
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start/Stop deadlocked")
	}
}
