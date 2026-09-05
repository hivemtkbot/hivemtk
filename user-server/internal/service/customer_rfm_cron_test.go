package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newTestRFMCron(fn func(ctx context.Context) (int, error)) *CustomerRFMCron {
	c := NewCustomerRFMCron(nil)
	c.computeFn = fn
	c.retryBackoff = []time.Duration{time.Microsecond, time.Microsecond, time.Microsecond}
	return c
}

// TestCustomerRFMCron_RunWithRetry_SuccessAfterRetries 前两次失败第三次成功：应重试而非告警
func TestCustomerRFMCron_RunWithRetry_SuccessAfterRetries(t *testing.T) {
	attempts := 0
	c := newTestRFMCron(func(ctx context.Context) (int, error) {
		attempts++
		if attempts < 3 {
			return attempts, errors.New("db transient error")
		}
		return 42, nil
	})
	alerted := false
	c.onFinalFailure = func(ctx context.Context, n int, err error) { alerted = true }

	c.runWithRetry(context.Background())

	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
	if alerted {
		t.Error("should not alert when a retry succeeds")
	}
}

// TestCustomerRFMCron_RunWithRetry_ExhaustedAlerts 全部失败：首跑+3 次重试后触发一次最终告警
func TestCustomerRFMCron_RunWithRetry_ExhaustedAlerts(t *testing.T) {
	attempts := 0
	c := newTestRFMCron(func(ctx context.Context) (int, error) {
		attempts++
		return 0, errors.New("permanent failure")
	})
	var alertAttempts int
	var alertErr error
	alertCount := 0
	c.onFinalFailure = func(ctx context.Context, n int, err error) {
		alertCount++
		alertAttempts = n
		alertErr = err
	}

	c.runWithRetry(context.Background())

	if attempts != 4 {
		t.Errorf("attempts = %d, want 4 (initial + 3 retries)", attempts)
	}
	if alertCount != 1 {
		t.Fatalf("alert count = %d, want 1", alertCount)
	}
	if alertAttempts != 4 {
		t.Errorf("alert attempts = %d, want 4", alertAttempts)
	}
	if alertErr == nil {
		t.Error("alert should carry the last error")
	}
}

// TestCustomerRFMCron_ComputeOnce_PanicRecovered 计算 panic 应被转为错误而非崩溃进程
func TestCustomerRFMCron_ComputeOnce_PanicRecovered(t *testing.T) {
	c := newTestRFMCron(func(ctx context.Context) (int, error) {
		panic("boom")
	})
	_, err := c.computeOnce(context.Background())
	if err == nil {
		t.Fatal("expected panic to be converted to error")
	}
}

// TestCustomerRFMCron_SleepInterruptedByStop 退避等待必须能被 Stop 中断（不阻塞关闭）
func TestCustomerRFMCron_SleepInterruptedByStop(t *testing.T) {
	c := NewCustomerRFMCron(nil)
	done := make(chan bool, 1)
	go func() { done <- c.sleep(time.Minute) }()
	time.Sleep(10 * time.Millisecond)
	c.Stop(context.Background())
	select {
	case ok := <-done:
		if ok {
			t.Error("sleep should return false after stop")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sleep was not interrupted by Stop")
	}
}
