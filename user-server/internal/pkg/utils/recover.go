package utils

import (
	"context"
	"runtime/debug"

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
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().
					Str("goroutine", name).
					Interface("panic", r).
					Bytes("stack", debug.Stack()).
					Msg("后台 goroutine panic 已被 recover，避免进程崩溃")
			}
		}()
		fn(ctx)
	}()
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
	go func() {
		defer func() {
			if r := recover(); r != nil {
				onPanic(name, r, debug.Stack())
			}
		}()
		fn(ctx)
	}()
}

func defaultOnPanic(name string, r interface{}, stack []byte) {
	log.Error().
		Str("goroutine", name).
		Interface("panic", r).
		Bytes("stack", stack).
		Msg("后台 goroutine panic 已被 recover，避免进程崩溃")
}

