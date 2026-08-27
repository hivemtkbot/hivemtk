package utils

// WarnErrIfNotNil 治理 M6：明确将历史上被吞掉的 err 写为 warn 日志。
// 用法：替代 `_ = doSomething()`，改为 `utils.WarnErrIfNotNil("module.func", doSomething())`。
//
// 设计：
//   - err == nil → 一条空函数调用，性能敏感路径可走 GetWarnLogger().Debug() 兜底；
//   - err != nil → 以 warn 级别写入（含 trace_id 由上游 logger hook 注入，参见 R38 M11）；
//   - 显式 context 仅在调用方有 ctx 时使用 WarnErrCtx；否则使用全局 logger。
//
// 测试：参见 warnlog_test.go。
func WarnErrIfNotNil(scope string, err error) {
	if err == nil {
		return
	}
	GetLogger().Warn().
		Str("scope", scope).
		Err(err).
		Msg("warn-only err (originally silently swallowed)")
}

// WarnErrCtx 治理 M6：带 ctx 的吞错 warn，ctx 用于注入 trace_id（即使 M11 尚未落
// 地 OTEL，全局 logger 仍会从 zerolog hook 拿 trace_id 输出格式）。
func WarnErrCtx(scope string, ctx zerologContext, err error) {
	if err == nil {
		return
	}
	GetLogger().Warn().
		Str("scope", scope).
		Interface("ctx_meta", ctxStringView(ctx)).
		Err(err).
		Msg("warn-only err with ctx")
}

// WarnErrKV 治理 M6：context-感知的吞错 warn，提供 KV 附加字段。
// 常见用法：模块名/账号ID/消息ID 等便于排查。
func WarnErrKV(scope string, err error, kvs ...string) {
	if err == nil {
		return
	}
	ev := GetLogger().Warn().Str("scope", scope).Err(err)
	for i := 0; i+1 < len(kvs); i += 2 {
		ev = ev.Str(kvs[i], kvs[i+1])
	}
	ev.Msg("warn-only err with kv")
}

// zerologContext 抽象：让 WarnErrCtx 不必直接依赖 gin/context，
// 调用方只传最小必要信息（trace_id / request_id）。
type zerologContext interface{}

// ctxStringView 把传入的 ctx 投影到 KV，减少调用方 import。
// 当前为预留——M11 trace_id 贯通时由 zerolog hook 自动注入；这里仅防御性实现。
func ctxStringView(_ zerologContext) map[string]string {
	out := make(map[string]string)
	// M11 落地后，从 ctx 读 trace_id 填充
	return out
}
