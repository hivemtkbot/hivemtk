package utils

func WarnErrIfNotNil(scope string, err error) {
	if err == nil {
		return
	}
	GetLogger().Warn().
		Str("scope", scope).
		Err(err).
		Msg("warn-only err (originally silently swallowed)")
}

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

type zerologContext interface{}

func ctxStringView(_ zerologContext) map[string]string {
	out := make(map[string]string)

	return out
}
