package service

import (
	"context"
	"testing"
)

// T-7 分桶确定性：同 (scriptID, oneID) 恒定同桶；分布覆盖两桶
func TestScriptAB_AssignBucket_Deterministic(t *testing.T) {
	svc := NewScriptABService(nil)
	cfg := DefaultScriptABConfig()

	b1 := svc.AssignBucket(7, "one-abc", cfg)
	for i := 0; i < 50; i++ {
		if got := svc.AssignBucket(7, "one-abc", cfg); got != b1 {
			t.Fatalf("分桶不粘性: want %s got %s", b1, got)
		}
	}

	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		oneID := "one-" + string(rune('a'+i%26)) + string(rune('0'+i/26%10)) + string(rune('x'+i/260%5))
		seen[svc.AssignBucket(9, oneID, cfg)] = true
	}
	if len(seen) < 2 {
		t.Fatalf("分桶应覆盖 A/B 两桶, got %v", seen)
	}
}

// T-7 配额边界：AssignBucket 确定性语义（SplitA=100 → 全 A）；越界钳制在 GetConfig/SaveConfig 层
func TestScriptAB_AssignBucket_ConfigBounds(t *testing.T) {
	svc := NewScriptABService(nil)
	cfg := DefaultScriptABConfig()
	cfg.SplitA = 100
	if got := svc.AssignBucket(1, "one-x", cfg); got != "A" {
		t.Fatalf("SplitA=100 时所有 key 应为 A, got %s", got)
	}
	cfg.SplitA = 1
	if got := svc.AssignBucket(1, "one-x", cfg); got != "B" {
		t.Fatalf("SplitA=1 时 hash%%100>=1 的 key 应为 B, got %s", got)
	}
}

// T-6 配置持久化缺省：无 KV 注入时回退默认 50/50/48h
func TestScriptAB_GetConfig_DefaultWithoutKV(t *testing.T) {
	svc := NewScriptABService(nil)
	cfg := svc.GetConfig(context.Background(), 42)
	if !cfg.Enabled || cfg.SplitA != 50 || cfg.AttributionH != ScriptABAttributionHours {
		t.Fatalf("默认配置不符: %+v", cfg)
	}
}

// T-6 SaveConfig 边界：split_a 越界拒绝
func TestScriptAB_SaveConfig_RejectsBadSplit(t *testing.T) {
	svc := NewScriptABService(nil)
	if err := svc.SaveConfig(context.Background(), 1, ScriptABConfig{Enabled: true, SplitA: 0}); err == nil {
		t.Fatal("split_a=0 应被拒绝")
	}
	if err := svc.SaveConfig(context.Background(), 1, ScriptABConfig{Enabled: true, SplitA: 100}); err == nil {
		t.Fatal("split_a=100 应被拒绝")
	}
}
