package utils

import (
	"strconv"

	"hivemtk-user/internal/pkg/utils/logger"
)

// ParseInt64OrZero 解析字符串为 int64，失败时返回 0 并 warn 一行；
// 用于"参数缺失或非法时业务降级到 0"的路径（如分页 size=0、分组查询默认 ID=0）。
//
// ⚠️ 严禁用于：
//   - URL 路径中的资源 ID（如 /users/:id） — 这类必须返回 400，否则会出现"路由命中但读到 0 行"的隐式 bug；
//   - 安全敏感参数（如 merchant_key） — 必须返回错误而不是静默 0。
//
// 之所以提供此工具，是因为业务里"非法 size 当 0 用"的场景多达 10+ 处，逐处
// 打 warn 会污染日志，集中在这里降级 + 一次性 warn 更易观测。
func ParseInt64OrZero(scope, raw string) int64 {
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		logger.Warnf("%s: parse_int64_or_zero_failed raw=%q fallback=0 hint=%s",
			scope, raw, "非法或溢出，fallback=0；若属业务关键路径请改用 ParseInt64Strict")
		return 0
	}
	return v
}

// ParseInt64Strict 解析字符串为 int64，失败返回 (0, err)；
// 用于路径参数、关键业务 ID 等必须上报错误的场景。
func ParseInt64Strict(scope, raw string) (int64, error) {
	if raw == "" {
		return 0, &ParamError{Scope: scope, Raw: raw, Reason: "empty"}
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, &ParamError{Scope: scope, Raw: raw, Reason: err.Error()}
	}
	return v, nil
}

// ParamError 严格解析失败的统一错误类型，便于 handler 层识别并返回 400。
type ParamError struct {
	Scope  string
	Raw    string
	Reason string
}

func (e *ParamError) Error() string {
	return "param[" + e.Scope + "] invalid: raw=" + e.Raw + " reason=" + e.Reason
}