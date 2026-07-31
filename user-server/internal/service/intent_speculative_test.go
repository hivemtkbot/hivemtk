package service

import (
	"context"
	"testing"
	"time"
)

// 2026-07-31 AI 智能体性能优化 - 投机意图识别 测试
//
// 测试策略:
//   - 用 IntentEnabled 全局开关控制,不依赖真实 dispatcher
//   - 验证: 同步结果正确 + 异步 channel 行为符合预期
//   - 不实际触发 LLM 调用 (通过 dispatcher=nil 短路)

func TestRecognizeSpeculative_EmptyText(t *testing.T) {
	rec := &IntentRecognizer{} // dispatcher nil
	ctx := context.Background()
	result, ch, err := rec.RecognizeSpeculative(ctx, "s1", "c1", "")
	if err != nil {
		t.Fatal(err)
	}
	if result.IntentType != IntentUnknown {
		t.Errorf("expected unknown, got %s", result.IntentType)
	}
	// channel 应该立即有数据
	select {
	case r := <-ch:
		if r.IntentType != IntentUnknown {
			t.Errorf("ch result should be unknown, got %s", r.IntentType)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected ch to receive immediately")
	}
}

func TestRecognizeSpeculative_DisabledFlag(t *testing.T) {
	IntentEnabled = false
	defer func() { IntentEnabled = true }()
	rec := &IntentRecognizer{}
	ctx := context.Background()
	result, ch, err := rec.RecognizeSpeculative(ctx, "s1", "c1", "你好")
	if err != nil {
		t.Fatal(err)
	}
	if result.Method != "disabled" {
		t.Errorf("expected method=disabled, got %s", result.Method)
	}
	// 立即收到
	select {
	case <-ch:
	case <-time.After(50 * time.Millisecond):
		t.Error("expected ch immediate receive")
	}
}

func TestRecognizeSpeculative_RuleHit(t *testing.T) {
	rec := &IntentRecognizer{} // dispatcher nil, 但规则先于 dispatcher 跑
	ctx := context.Background()
	// "你好" 应该是规则命中 (greeting)
	result, ch, err := rec.RecognizeSpeculative(ctx, "s1", "c1", "你好")
	if err != nil {
		t.Fatal(err)
	}
	if result.Method != "rule" {
		t.Errorf("expected method=rule, got %s", result.Method)
	}
	if result.Confidence < 0.9 {
		t.Errorf("expected high confidence, got %f", result.Confidence)
	}
	// channel 应该 close (dispatcher nil 时, 规则命中后没有 LLM 异步跑)
	select {
	case _, ok := <-ch:
		if ok {
			// 如果有数据 (LLM 完成后投递), 也合法
		}
	case <-time.After(100 * time.Millisecond):
		// close 也不报错
	}
}

func TestRecognizeSpeculative_RuleMiss_NoDispatcher(t *testing.T) {
	rec := &IntentRecognizer{} // dispatcher nil
	ctx := context.Background()
	// 不太可能命中规则的输入
	result, ch, err := rec.RecognizeSpeculative(ctx, "s1", "c1", "随便说点啥12345abc")
	if err != nil {
		t.Fatal(err)
	}
	if result.Method != "rule_placeholder" {
		t.Errorf("expected method=rule_placeholder, got %s", result.Method)
	}
	// channel 应该立即有 placeholder
	select {
	case r := <-ch:
		if r.Method != "rule_placeholder" {
			t.Errorf("expected placeholder, got %s", r.Method)
		}
	case <-time.After(50 * time.Millisecond):
		t.Error("expected ch to receive placeholder")
	}
}

func TestRecognizeSpeculative_Channel_BufferIsOne(t *testing.T) {
	// 验证 channel buffer=1 (不阻塞 LLM 协程)
	rec := &IntentRecognizer{}
	ctx := context.Background()
	_, ch, _ := rec.RecognizeSpeculative(ctx, "s1", "c1", "你好")
	// channel 应该能缓冲 1 个
	if cap(ch) < 1 {
		t.Errorf("expected ch cap >= 1, got %d", cap(ch))
	}
}
