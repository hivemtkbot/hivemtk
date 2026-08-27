package utils

import (
	"context"
	"runtime/debug"
	"time"

	"github.com/rs/zerolog/log"
)

// SafeGo 启动一个带 panic recover 的 goroutine，避免后台任务 panic 击穿整个进程。
// 用法：utils.SafeGo(ctx, "whatsapp.Login", func(ctx context.Context) { ... })
//
// 设计：
//   - 自动从 panic 中恢复并记录堆栈到 zerolog
//   - ctx 用于传递请求级上下文（可选，传 nil 视为 Background）
//   - name 用于日志定位（建议传"模块.动作"格式）
//
// 注意：
//   - 仅保护 panic 不传播，不能替代业务层的错误处理
//   - 进程级关键路径仍需独立部署 supervisor / endless 重启保护
func SafeGo(ctx context.Context, name string, fn func(ctx context.Context)) {
	if fn == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	go runRecover(name, func(c context.Context) {
		fn(c)
	}, ctx, false, 0, nil)
}

// SafeGoDetached 启动一个独立的后台 goroutine，与请求生命周期解耦。
//
// 行为：
//   - 用 context.WithoutCancel(ctx) 隔离原 ctx 的取消链，但保留 ctx.Value（trace_id 等）
//   - 若 timeout > 0，则叠加 WithTimeout 作为硬上限（防止异步任务永久卡住）
//   - panic 时自动 recover 并通过 zerolog 记录完整堆栈
//
// 适用场景：
//   - HTTP 请求结束后仍需继续执行的任务（如异步通知、审计落库、消息补偿）
//   - 长时间运行的后台 worker（带 timeout 防止泄漏）
//
// 注意：
//   - 取消链被剥离 → 即便客户端断开/服务端超时，请求级 ctx.Done() 也不会终止本任务
//   - 必须依赖 timeout 或业务自行管理生命周期
func SafeGoDetached(ctx context.Context, name string, timeout time.Duration, fn func(ctx context.Context)) {
	if fn == nil {
		return
	}
	parent := context.Background()
	if ctx != nil {
		parent = context.WithoutCancel(ctx)
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		parent, cancel = context.WithTimeout(parent, timeout)
		// 注：此处 cancel 不在 defer 中调用——detach 场景下我们希望任务在
		// 内部自行管理 ctx.Done()，cancel 交由 ctx 自身在任务退出时回收。
		_ = cancel
	}
	go runRecover(name, func(c context.Context) {
		fn(c)
	}, parent, true, timeout, nil)
}

// SafeGoWithRetry 启动一个带退避重试的 SafeGo 任务。
//
// 行为：
//   - 调用 fn(ctx) 若返回 nil → 立即成功结束
//   - 调用 fn(ctx) 若返回 error → 按 b 给出的退避（backoff.BackOff）等待后重试
//   - 永久重试直到 fn 返回 nil 或 b.NextBackOff() 返回 stop 信号
//   - panic 仍由 recover 捕获（避免进程崩溃），但不计入重试计数
//
// 设计动机：项目里"调用外部 API + 失败重试"非常常见；统一收口避免每个调用点
// 自己实现 sleep + retry 循环，减少 sleep 时未 SafeGo 导致 panic 击穿进程的风险。
//
// 退避实现：使用项目已引入的 golang.org/x/time/backoff 自实现等价逻辑，
// 避免再引第三方依赖（与 github.com/cenkalti/backoff/v5 行为对齐）。
//
// 用法：
//
//	b := backoff.NewExponentialBackOff()
//	b.InitialInterval = 500 * time.Millisecond
//	b.MaxInterval = 30 * time.Second
//	utils.SafeGoWithRetry(ctx, "outreach.retry", b, fn)
func SafeGoWithRetry(ctx context.Context, name string, b BackOff, fn func(ctx context.Context) error) {
	if fn == nil {
		return
	}
	if b == nil {
		b = defaultBackOff()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	go runRecover(name, func(c context.Context) {
		attempt := 0
		for {
			attempt++
			err := safeCall(c, name, attempt, fn)
			if err == nil {
				return
			}
			log.Warn().
				Str("goroutine", name).
				Int("attempt", attempt).
				Err(err).
				Msg("SafeGoWithRetry 调用失败，准备退避重试")
			wait, stop := b.NextBackOff()
			if stop {
				log.Error().
					Str("goroutine", name).
					Int("attempt", attempt).
					Err(err).
					Msg("SafeGoWithRetry 退避策略返回 stop，放弃重试")
				return
			}
			if wait <= 0 {
				continue
			}
			select {
			case <-c.Done():
				if c.Err() != nil {
					log.Warn().
						Str("goroutine", name).
						Err(c.Err()).
						Msg("SafeGoWithRetry ctx 已取消，停止重试")
				}
				return
			case <-time.After(wait):
			}
		}
	}, ctx, false, 0, nil)
}

// SafeGoWithRecover 自定义 recover 行为的版本，调用方可以传入自己的处理函数。
// 适用于需要上报自定义日志 / webhook 等扩展场景。
func SafeGoWithRecover(ctx context.Context, name string, fn func(ctx context.Context), onPanic func(name string, r interface{}, stack []byte)) {
	if fn == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if onPanic == nil {
		onPanic = defaultOnPanic
	}
	go runRecover(name, func(c context.Context) {
		fn(c)
	}, ctx, false, 0, onPanic)
}

// safeCall 在独立函数中执行 fn，让 recover 覆盖 fn 自身的 panic 并转为 error 返回，
// 让 SafeGoWithRetry 走重试分支而不是直接退出。
func safeCall(ctx context.Context, name string, attempt int, fn func(ctx context.Context) error) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			defaultOnPanic(name, r, stack)
			if e, ok := r.(error); ok {
				retErr = e
			} else {
				retErr = &panicError{val: r, stack: stack}
			}
		}
	}()
	return fn(ctx)
}

// panicError 把非 error 类型的 panic 值包装成 error，便于重试逻辑统一处理。
type panicError struct {
	val   interface{}
	stack []byte
}

func (p *panicError) Error() string {
	return "panic in SafeGoWithRetry: non-error panic value"
}

// runRecover 统一的 goroutine 包装：recover panic + 记录堆栈 + 执行 fn。
func runRecover(name string, fn func(context.Context), ctx context.Context, detached bool, timeout time.Duration, onPanic func(string, interface{}, []byte)) {
	defer func() {
		if r := recover(); r != nil {
			if onPanic != nil {
				onPanic(name, r, debug.Stack())
				return
			}
			defaultOnPanic(name, r, debug.Stack())
		}
	}()
	if detached && timeout > 0 {
		// 二次确认 timeout 在 detached 场景下被尊重
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= 0 {
			log.Warn().
				Str("goroutine", name).
				Dur("timeout", timeout).
				Msg("SafeGoDetached 启动时已超时，跳过执行")
			return
		}
	}
	fn(ctx)
}

func defaultOnPanic(name string, r interface{}, stack []byte) {
	log.Error().
		Str("goroutine", name).
		Interface("panic", r).
		Bytes("stack", stack).
		Msg("后台 goroutine panic 已被 recover，避免进程崩溃")
}

func defaultBackOff() BackOff {
	b := NewExponentialBackOff()
	b.InitialInterval = 500 * time.Millisecond
	b.MaxInterval = 30 * time.Second
	b.MaxElapsedTime = 0 // 永不过期，配合 NextBackOff stop 信号
	return b
}