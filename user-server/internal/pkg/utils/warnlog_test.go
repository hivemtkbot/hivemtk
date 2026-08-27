package utils

import (
	"errors"
	"strings"
	"testing"
)

func TestWarnErrIfNotNil_NilDoesNothing(t *testing.T) {
	// 无副作用——确认 nil err 不写出错或调用底层 logger 异常。
	WarnErrIfNotNil("test.scope", nil)
}

func TestWarnErrIfNotNil_NonNilLogsWarn(t *testing.T) {
	// 仅为烟雾测试：吞掉的 err 走 warn 而非 Error，避免 SLO 报警噪音。
	// 真要断言 logger 输出，需引入自定义 hook；本测试仅保证调用不 panic。
	WarnErrIfNotNil("test.scope", errors.New("simulated swallow"))
}

func TestWarnErrKV_NilDoesNothing(t *testing.T) {
	WarnErrKV("test.scope", nil)
	WarnErrKV("test.scope", nil, "k1", "v1", "k2", "v2")
}

func TestWarnErrKV_OddKVIgnoresTail(t *testing.T) {
	// 即使 kvs 数量为奇数也不应 panic。
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("odd KV should not panic, got %v", r)
		}
	}()
	WarnErrKV("test.scope", errors.New("err"), "k1", "v1", "dangling")
}

func TestWarnErrCtx_NilAndError(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil ctx should not panic, got %v", r)
		}
	}()
	WarnErrCtx("test.scope", nil, nil)
	WarnErrCtx("test.scope", nil, errors.New("err"))
}

func TestWarnErrIfNotNil_QuietNotSuffix(t *testing.T) {
	// 防御：scope 不能包含换行 / 控制字符造成日志注入。
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("weird scope should not panic, got %v", r)
		}
	}()
	WarnErrIfNotNil("weird\nscope\twith\rinjection", errors.New("err"))
	WarnErrIfNotNil(strings.Repeat("x", 1024), errors.New("err"))
}
