package utils

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestSafeGo_Normal(t *testing.T) {
	var executed atomic.Bool
	SafeGo(context.Background(), "test.normal", func(ctx context.Context) {
		executed.Store(true)
	})
	// 等待 goroutine 启动
	time.Sleep(50 * time.Millisecond)
	if !executed.Load() {
		t.Fatal("SafeGo 未执行目标函数")
	}
}

func TestSafeGo_PanicRecovered(t *testing.T) {
	// 启动会 panic 的 goroutine，验证进程不被击穿
	SafeGo(context.Background(), "test.panic", func(ctx context.Context) {
		panic("test panic - 进程应存活")
	})
	// 给 recover 一点时间
	time.Sleep(50 * time.Millisecond)
	// 如果 recover 失败，进程已经崩溃，测试将无法继续
}

func TestSafeGo_NilFunc(t *testing.T) {
	// nil 函数不能 panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SafeGo(nil) 不应 panic, 实际: %v", r)
		}
	}()
	SafeGo(context.Background(), "test.nil", nil)
}

func TestSafeGo_NilCtx(t *testing.T) {
	// nil ctx 应回退到 Background
	var executed atomic.Bool
	SafeGo(nil, "test.nilctx", func(ctx context.Context) {
		if ctx == nil {
			t.Fatal("nil ctx 应被替换为 Background")
		}
		executed.Store(true)
	})
	time.Sleep(50 * time.Millisecond)
	if !executed.Load() {
		t.Fatal("SafeGo(nil ctx) 未执行目标函数")
	}
}

func TestSafeGoWithRecover_CustomHandler(t *testing.T) {
	var called atomic.Bool
	SafeGoWithRecover(context.Background(), "test.custom", func(ctx context.Context) {
		panic(errors.New("custom panic"))
	}, func(name string, r interface{}, stack []byte) {
		called.Store(true)
		if name != "test.custom" {
			t.Errorf("onPanic 收到的 name = %q, 期望 test.custom", name)
		}
		if r == nil {
			t.Error("onPanic 收到的 panic 值为 nil")
		}
		if len(stack) == 0 {
			t.Error("onPanic 收到的 stack 为空")
		}
	})
	time.Sleep(50 * time.Millisecond)
	if !called.Load() {
		t.Fatal("onPanic 回调未被调用")
	}
}
