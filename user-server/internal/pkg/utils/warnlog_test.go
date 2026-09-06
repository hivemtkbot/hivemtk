package utils

import (
	"errors"
	"strings"
	"testing"
)

func TestWarnErrIfNotNil_NilDoesNothing(t *testing.T) {

	WarnErrIfNotNil("test.scope", nil)
}

func TestWarnErrIfNotNil_NonNilLogsWarn(t *testing.T) {

	WarnErrIfNotNil("test.scope", errors.New("simulated swallow"))
}

func TestWarnErrKV_NilDoesNothing(t *testing.T) {
	WarnErrKV("test.scope", nil)
	WarnErrKV("test.scope", nil, "k1", "v1", "k2", "v2")
}

func TestWarnErrKV_OddKVIgnoresTail(t *testing.T) {

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

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("weird scope should not panic, got %v", r)
		}
	}()
	WarnErrIfNotNil("weird\nscope\twith\rinjection", errors.New("err"))
	WarnErrIfNotNil(strings.Repeat("x", 1024), errors.New("err"))
}
