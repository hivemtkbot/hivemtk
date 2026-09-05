package service

import (
	"testing"

	"hivemtk-user/internal/service/humanize/behavioral"
)

// TestBehavioralPlanBuilder_DisabledByDefault 验证默认禁用
func TestBehavioralPlanBuilder_DisabledByDefault(t *testing.T) {
	b := NewBehavioralPlanBuilder()
	if b.IsEnabled() {
		t.Error("behavioral humanize should be disabled by default (A/B 灰度)")
	}
}

// TestBehavioralPlanBuilder_SetEnabled 验证启用开关
func TestBehavioralPlanBuilder_SetEnabled(t *testing.T) {
	b := NewBehavioralPlanBuilder()
	b.SetEnabled(true)
	if !b.IsEnabled() {
		t.Error("SetEnabled(true) should enable")
	}
	b.SetEnabled(false)
	if b.IsEnabled() {
		t.Error("SetEnabled(false) should disable")
	}
}

// TestBehavioralPlanBuilder_NilSafe 验证 nil 安全
func TestBehavioralPlanBuilder_NilSafe(t *testing.T) {
	var b *BehavioralPlanBuilder
	b.SetEnabled(true)
	if b.IsEnabled() {
		t.Error("nil should be disabled")
	}
}

// TestBehavioralPlanBuilder_DisabledReturnsTrivialPlan 验证禁用时返回单条消息
func TestBehavioralPlanBuilder_DisabledReturnsTrivialPlan(t *testing.T) {
	b := NewBehavioralPlanBuilder()

	longText := "这是第一段。这是第二段。这是第三段。"
	plan := b.Build(longText, true)
	if len(plan.Messages) != 1 {
		t.Errorf("disabled should yield 1 message, got %d", len(plan.Messages))
	}
	if plan.Messages[0] != longText {
		t.Errorf("text mismatch: got %q want %q", plan.Messages[0], longText)
	}
}

// TestBehavioralPlanBuilder_EnabledBuildsPlan 验证启用时分条
func TestBehavioralPlanBuilder_EnabledBuildsPlan(t *testing.T) {
	b := NewBehavioralPlanBuilder()
	b.SetEnabled(true)

	longText := "您好！我是智能助手，可以帮您处理订单查询、物流跟踪、退款申请、售后服务等常见问题。请您简要描述下您当前遇到的具体情况。我会立即为您查询相关数据并提供详细的解决方案，期待您的回复！祝您生活愉快！"
	plan := b.Build(longText, true)
	if len(plan.Messages) < 2 {
		t.Errorf("enabled with long text should split, got %d messages", len(plan.Messages))
	}
}

// TestBehavioralPlanBuilder_NilSafeBuild 验证 nil Build
func TestBehavioralPlanBuilder_NilSafeBuild(t *testing.T) {
	var b *BehavioralPlanBuilder
	plan := b.Build("test", true)
	if plan.Messages == nil || len(plan.Messages) != 1 {
		t.Errorf("nil builder should return trivial plan")
	}
}

// TestBehavioralPlanBuilder_SetConfig 验证配置
func TestBehavioralPlanBuilder_SetConfig(t *testing.T) {
	b := NewBehavioralPlanBuilder()
	cfg := behavioral.BehaviorConfig{
		EnableTypingDelay:   true,
		TypingSpeedCPS:      10.0,
		ThinkingPauseSec:    5.0,
		EnableMessageSplit:  false,
		SplitThresholdChars: 80,
		SplitMinIntervalSec: 1.0,
		EnableTypoInjection: false,
		TypoProbability:     0.0,
	}
	b.SetConfig(cfg)
	b.SetEnabled(true)

	plan := b.Build("short text", true)
	if len(plan.Messages) != 1 {
		t.Errorf("split disabled should yield 1 message, got %d", len(plan.Messages))
	}
}

// TestIsFirstMessageOf 验证首条消息判定
func TestIsFirstMessageOf(t *testing.T) {
	tests := []struct {
		name string
		req  *SalesRequest
		want bool
	}{
		{"nil request", nil, false},
		{"first turn", &SalesRequest{IsFirstTurn: true}, true},
		{"not first", &SalesRequest{IsFirstTurn: false}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isFirstMessageOf(tt.req); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
