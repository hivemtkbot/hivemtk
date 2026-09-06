package featureflag

import (
	"os"
	"testing"
	"time"
)

func refreshAll() { DefaultManager().ReloadAll() }

func TestFlag_DefaultFalse(t *testing.T) {
	os.Unsetenv("FF_PARALLEL")
	refreshAll()
	if Get("parallel").Bool() {
		t.Error("expected default false")
	}
}

func TestFlag_EnvTrue(t *testing.T) {
	os.Setenv("FF_PARALLEL", "1")
	defer os.Unsetenv("FF_PARALLEL")
	refreshAll()
	if !Get("parallel").Bool() {
		t.Error("expected true from env=1")
	}
}

func TestFlag_EnvFalse(t *testing.T) {
	os.Setenv("FF_PARALLEL", "0")
	defer os.Unsetenv("FF_PARALLEL")
	refreshAll()
	if Get("parallel").Bool() {
		t.Error("expected false from env=0")
	}
}

func TestFlag_EnvTrueWord(t *testing.T) {
	tests := []string{"true", "True", "TRUE", "yes", "YES", "on"}
	for _, v := range tests {
		os.Setenv("FF_LAYER1", v)
		refreshAll()
		if !Get("layer1").Bool() {
			t.Errorf("expected true from env=%s", v)
		}
		os.Unsetenv("FF_LAYER1")
	}
	refreshAll()
}

func TestFlag_EnvFalseWord(t *testing.T) {
	tests := []string{"false", "False", "no", "NO", "off"}
	for _, v := range tests {
		os.Setenv("FF_LAYER1", v)
		refreshAll()
		if Get("layer1").Bool() {
			t.Errorf("expected false from env=%s", v)
		}
		os.Unsetenv("FF_LAYER1")
	}
	refreshAll()
}

func TestFlag_UnknownDefault(t *testing.T) {
	os.Unsetenv("FF_NONEXISTENT")
	refreshAll()
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
	refreshAll()
	snap := AllFlagSnapshot()
	if !snap["parallel"] {
		t.Error("snapshot parallel should be true")
	}
	if snap["stream"] {
		t.Error("snapshot stream should be false")
	}
}

func TestFlag_DefaultManager_6Flags(t *testing.T) {
	for _, name := range []string{"parallel", "stream", "layer1", "fallback_chain", "debug_log", "sse_bridge"} {
		f := Get(name)
		if f == nil {
			t.Errorf("flag %s should be registered", name)
		}
	}
}

// TestFlag_HotReload 验证 : env 变更后 5s 内自动热加载 (后台轮询)
//
// 测试流程:
//  1. 初始 state: flag=false (env unset)
//  2. 设置 env FF_<NAME>=1
//  3. 等待 5s (一次 poll 周期)
//  4. 验证 flag 自动变为 true (无需重启进程)
func TestFlag_HotReload(t *testing.T) {
	flagName := "test_hot_reload"
	envName := "FF_TEST_HOT_RELOAD"

	os.Unsetenv(envName)

	f := Get(flagName)
	if f.Bool() {
		t.Fatal("expected initial false before setting env")
	}

	before := f.LastReload()

	DefaultManager().ReloadAll()
	if f.Bool() {
		t.Error("expected false after ReloadAll (env unset)")
	}

	os.Setenv(envName, "1")
	DefaultManager().ReloadAll()
	if !f.Bool() {
		t.Error("expected true after ReloadAll with env=1")
	}

	os.Setenv(envName, "0")
	if !f.Bool() {
		t.Error("expected cached true before poll (5s 内不应失效)")
	}

	t.Logf("waiting up to 6s for background poller to refresh...")
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		if !f.Bool() {
			if !f.LastReload().After(before) {
				t.Error("LastReload should advance after poller ran")
			}
			t.Logf("✅ hot reload OK, lastReload advanced: %s -> %s", before.Format(time.RFC3339Nano), f.LastReload().Format(time.RFC3339Nano))
			goto done
		}
	}
	t.Fatal("hot reload did not occur within 6s (expected 5s poll interval)")

done:
	os.Unsetenv(envName)
	DefaultManager().ReloadAll()
	if f.Bool() {
		t.Error("expected false after ReloadAll with env unset")
	}
}

// TestFlag_Resolve_Immediate 测试 resolve() 立即读 env (admin API 场景)
func TestFlag_Resolve_Immediate(t *testing.T) {
	flagName := "test_resolve_immediate"
	envName := "FF_TEST_RESOLVE_IMMEDIATE"
	os.Unsetenv(envName)

	f := Get(flagName)
	if f.Bool() {
		t.Fatal("expected initial false")
	}

	os.Setenv(envName, "1")
	if !f.resolve() {
		t.Error("resolve() should immediately read env=1")
	}
	if !f.Bool() {
		t.Error("Bool() should reflect the resolved value (cached)")
	}

	os.Unsetenv(envName)
	if f.resolve() {
		t.Error("resolve() should immediately read env unset -> defaultValue")
	}
	if f.Bool() {
		t.Error("Bool() should reflect new false after resolve()")
	}
}

// TestFlag_ReloadAll_UpdateLastReload 验证 ReloadAll 更新 lastReload
func TestFlag_ReloadAll_UpdateLastReload(t *testing.T) {
	flagName := "test_reload_ts"
	os.Unsetenv("FF_TEST_RELOAD_TS")

	f := Get(flagName)
	before := f.LastReload()

	time.Sleep(10 * time.Millisecond)
	DefaultManager().ReloadAll()

	after := f.LastReload()
	if !after.After(before) {
		t.Errorf("LastReload should advance: before=%s after=%s", before, after)
	}
}
