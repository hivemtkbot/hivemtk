package featureflag

import (
	"os"
	"testing"
)

func TestFlag_DefaultFalse(t *testing.T) {
	// 未设置 env, 默认 false
	os.Unsetenv("FF_PARALLEL")
	if Get("parallel").Bool() {
		t.Error("expected default false")
	}
}

func TestFlag_EnvTrue(t *testing.T) {
	os.Setenv("FF_PARALLEL", "1")
	defer os.Unsetenv("FF_PARALLEL")
	if !Get("parallel").Bool() {
		t.Error("expected true from env=1")
	}
}

func TestFlag_EnvFalse(t *testing.T) {
	os.Setenv("FF_PARALLEL", "0")
	defer os.Unsetenv("FF_PARALLEL")
	if Get("parallel").Bool() {
		t.Error("expected false from env=0")
	}
}

func TestFlag_EnvTrueWord(t *testing.T) {
	tests := []string{"true", "True", "TRUE", "yes", "YES", "on"}
	for _, v := range tests {
		os.Setenv("FF_LAYER1", v)
		if !Get("layer1").Bool() {
			t.Errorf("expected true from env=%s", v)
		}
		os.Unsetenv("FF_LAYER1")
	}
}

func TestFlag_EnvFalseWord(t *testing.T) {
	tests := []string{"false", "False", "no", "NO", "off"}
	for _, v := range tests {
		os.Setenv("FF_LAYER1", v)
		if Get("layer1").Bool() {
			t.Errorf("expected false from env=%s", v)
		}
		os.Unsetenv("FF_LAYER1")
	}
}

func TestFlag_UnknownDefault(t *testing.T) {
	os.Unsetenv("FF_NONEXISTENT")
	if Get("nonexistent").Bool() {
		t.Error("expected default false for unknown flag")
	}
}

func TestFlag_AllSnapshot(t *testing.T) {
	os.Setenv("FF_PARALLEL", "1")
	os.Setenv("FF_STREAM", "0")
	defer func() {
		os.Unsetenv("FF_PARALLEL")
		os.Unsetenv("FF_STREAM")
	}()
	snap := AllFlagSnapshot()
	if !snap["parallel"] {
		t.Error("snapshot parallel should be true")
	}
	if snap["stream"] {
		t.Error("snapshot stream should be false")
	}
}

func TestFlag_DefaultManager_5Flags(t *testing.T) {
	// 5 个核心开关必须存在
	for _, name := range []string{"parallel", "stream", "layer1", "fallback_chain", "debug_log"} {
		f := Get(name)
		if f == nil {
			t.Errorf("flag %s should be registered", name)
		}
	}
}

