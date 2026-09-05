package behavioral

import (
	"math"
	"math/rand"
	"strings"
	"testing"
)

// TestPlanSend_ShortText 验证短文本不分条
func TestPlanSend_ShortText(t *testing.T) {
	cfg := DefaultBehaviorConfig()
	cfg.SplitThresholdChars = 80
	plan := PlanSend("你好", cfg, true, nil)
	if len(plan.Messages) != 1 {
		t.Errorf("short text should not split, got %d messages", len(plan.Messages))
	}
	if plan.Messages[0] != "你好" {
		t.Errorf("text mismatch: %q", plan.Messages[0])
	}
	if len(plan.Intervals) != 0 {
		t.Error("single message should have no intervals")
	}
}

// TestPlanSend_LongText 验证长文本按句末标点分条
func TestPlanSend_LongText(t *testing.T) {
	cfg := DefaultBehaviorConfig()
	cfg.EnableTypingDelay = false

	text := "您好！我是智能助手，可以帮您处理订单查询、物流跟踪、退款申请、售后服务等常见问题。请您简要描述下您当前遇到的具体情况。我会立即为您查询相关数据并提供详细的解决方案，期待您的回复！祝您生活愉快！"
	if len([]rune(text)) <= cfg.SplitThresholdChars {
		t.Fatalf("test text must be > %d chars, got %d", cfg.SplitThresholdChars, len([]rune(text)))
	}
	plan := PlanSend(text, cfg, true, rand.New(rand.NewSource(1)))
	if len(plan.Messages) < 2 {
		t.Errorf("long text should split, got %d messages: %v", len(plan.Messages), plan.Messages)
	}

	reconstructed := strings.Join(plan.Messages, "")
	if reconstructed != text {
		t.Errorf("reconstruction mismatch:\n  got:  %q\n  want: %q", reconstructed, text)
	}
}

// TestPlanSend_FirstMessage_NoThinkingPause 验证首条消息无思考停顿
func TestPlanSend_FirstMessage_NoThinkingPause(t *testing.T) {
	cfg := DefaultBehaviorConfig()
	cfg.EnableTypingDelay = true
	cfg.ThinkingPauseSec = 5.0
	cfg.TypingSpeedCPS = 100
	plan := PlanSend("你", cfg, true, nil)
	if plan.TotalDelay > 0.5 {
		t.Errorf("first message should not have thinking pause, got delay=%v", plan.TotalDelay)
	}
}

// TestPlanSend_Continuation_HasThinkingPause 验证接续消息有思考停顿
func TestPlanSend_Continuation_HasThinkingPause(t *testing.T) {
	cfg := DefaultBehaviorConfig()
	cfg.EnableTypingDelay = true
	cfg.ThinkingPauseSec = 3.0
	cfg.TypingSpeedCPS = 1000
	plan := PlanSend("x", cfg, false, nil)
	if plan.TotalDelay < 2.5 || plan.TotalDelay > 3.5 {
		t.Errorf("continuation should have thinking pause ~3s, got %v", plan.TotalDelay)
	}
}

// TestPlanSend_SplitInterval_Respected 验证分条间隔 ≥ 最小值
func TestPlanSend_SplitInterval_Respected(t *testing.T) {
	cfg := DefaultBehaviorConfig()
	cfg.EnableTypingDelay = true
	cfg.SplitMinIntervalSec = 2.0
	cfg.TypingSpeedCPS = 10000
	text := "第一句。第二句！"
	plan := PlanSend(text, cfg, true, rand.New(rand.NewSource(42)))
	if len(plan.Messages) >= 2 {
		for i, iv := range plan.Intervals {
			if iv < 1.6 || iv > 2.4 {
				t.Errorf("interval[%d]=%v out of range [1.6, 2.4]", i, iv)
			}
		}
	}
}

// TestPlanSend_EmptyText 验证空文本
func TestPlanSend_EmptyText(t *testing.T) {
	plan := PlanSend("", DefaultBehaviorConfig(), true, nil)
	if len(plan.Messages) != 0 {
		t.Errorf("empty text should yield no messages, got %d", len(plan.Messages))
	}
	if plan.TotalDelay != 0 {
		t.Errorf("empty text should have no delay, got %v", plan.TotalDelay)
	}
}

// TestPlanSend_DisabledSplit 验证禁用分条时不分
func TestPlanSend_DisabledSplit(t *testing.T) {
	cfg := DefaultBehaviorConfig()
	cfg.EnableMessageSplit = false
	text := "第一句。第二句！第三句？"
	plan := PlanSend(text, cfg, true, nil)
	if len(plan.Messages) != 1 {
		t.Errorf("disabled split should yield 1 message, got %d", len(plan.Messages))
	}
}

// TestShouldSplit 验证分条判定
func TestShouldSplit(t *testing.T) {
	if shouldSplit("短文本", 80) {
		t.Error("text < threshold should not split")
	}
	if !shouldSplit(strings.Repeat("x", 81), 80) {
		t.Error("text > threshold should split")
	}
}

// TestSplitByPunctuation_Merge 验证过短段合并
func TestSplitByPunctuation_Merge(t *testing.T) {

	text := "你好。x。"
	segments := splitByPunctuation(text, rand.New(rand.NewSource(1)))
	for _, s := range segments {
		if len([]rune(s)) < 2 {
			t.Errorf("segment %q too short, should be merged", s)
		}
	}
}

// TestSplitByPunctuation_RoundTrip 验证分段后重组等于原文
func TestSplitByPunctuation_RoundTrip(t *testing.T) {
	tests := []string{
		"第一句。第二句！",
		"问句？答句。",
		"Hello. World!",
		"中文，逗号；分号。",
	}
	for _, text := range tests {
		segments := splitByPunctuation(text, rand.New(rand.NewSource(1)))
		got := strings.Join(segments, "")
		if got != text {
			t.Errorf("round trip failed:\n  in:  %q\n  out: %q\n  seg: %v", text, got, segments)
		}
	}
}

// TestTypingTime 验证打字时间计算
func TestTypingTime(t *testing.T) {
	if dt := typingTime("hello", 25.0); math.Abs(dt-0.2) > 1e-9 {
		t.Errorf("5 chars at 25 cps should be 0.2s, got %v", dt)
	}

	if dt := typingTime("hello", 0); dt <= 0 {
		t.Error("zero speed should fall back to 25 cps")
	}

	if dt := typingTime("hello", -1); dt <= 0 {
		t.Error("negative speed should fall back to 25 cps")
	}
}

// TestInjectTypos_ProbabilityZero 验证 prob=0 时不注入
func TestInjectTypos_ProbabilityZero(t *testing.T) {
	text := "hello world"
	out := injectTypos(text, 0, rand.New(rand.NewSource(1)))
	if out != text {
		t.Errorf("prob=0 should be no-op, got %q", out)
	}
}

// TestInjectTypos_AllAscii 验证 ASCII 字母被随机替换
func TestInjectTypos_AllAscii(t *testing.T) {
	text := "aaaaaaaaaa"
	rng := rand.New(rand.NewSource(1))
	changed := 0
	for i := 0; i < 100; i++ {
		out := injectTypos(text, 1.0, rng)
		if out != text {
			changed++

			for _, r := range out {
				if r != 'a' && r != 's' {
					t.Errorf("expected a or s, got %q", r)
				}
			}
		}
	}
	if changed == 0 {
		t.Error("prob=1.0 should always change, but got 0 changes in 100 trials")
	}
}

// TestInjectTypos_NoChineseChange 验证中文字符不被改
func TestInjectTypos_NoChineseChange(t *testing.T) {
	text := "你好世界"
	out := injectTypos(text, 1.0, rand.New(rand.NewSource(1)))
	if out != text {
		t.Errorf("Chinese chars should not be modified, got %q", out)
	}
}

// TestPlanSend_DeterministicWithFixedSeed 验证同 seed 产出一致结果（便于 A/B 测试）
func TestPlanSend_DeterministicWithFixedSeed(t *testing.T) {
	cfg := DefaultBehaviorConfig()
	cfg.EnableTypingDelay = false
	cfg.EnableTypoInjection = false
	text := "第一句。第二句！第三句？"

	rng1 := rand.New(rand.NewSource(42))
	rng2 := rand.New(rand.NewSource(42))
	p1 := PlanSend(text, cfg, true, rng1)
	p2 := PlanSend(text, cfg, true, rng2)

	if len(p1.Messages) != len(p2.Messages) {
		t.Errorf("deterministic: got %d vs %d messages", len(p1.Messages), len(p2.Messages))
	}
	for i := range p1.Messages {
		if p1.Messages[i] != p2.Messages[i] {
			t.Errorf("message[%d] mismatch: %q vs %q", i, p1.Messages[i], p2.Messages[i])
		}
	}
}

// TestPlanSend_TypingDelayScalesWithLength 验证延迟随文本长度增长
func TestPlanSend_TypingDelayScalesWithLength(t *testing.T) {
	cfg := DefaultBehaviorConfig()
	cfg.EnableTypingDelay = true
	cfg.TypingSpeedCPS = 25.0
	cfg.ThinkingPauseSec = 0
	cfg.EnableMessageSplit = false
	short := PlanSend("短", cfg, true, nil)
	long := PlanSend("这是一段非常长的文本用来测试打字时间是否随长度线性增长", cfg, true, nil)
	if long.TotalDelay <= short.TotalDelay {
		t.Errorf("longer text should have longer delay: short=%v long=%v",
			short.TotalDelay, long.TotalDelay)
	}
}
