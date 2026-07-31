package metrics

// ai_agent_test.go AI 智能体性能指标测试
//
// 2026-07-31 AI 智能体性能优化 (T21)

import "testing"

func TestRecordAIAgentWallTime(t *testing.T) {
	// 1. 记录 1 个 wall time 样本
	RecordAIAgentWallTime("ai_sales", "layer2", "greeting", 1.234)

	// 2. Range 应能读出 1 个 (sum=1.234, count=1)
	found := 0
	var observedSum float64
	GlobalMetrics.AIAgentWallTime.Range(func(labels string, sum float64, count uint64) {
		if labels == "ai_sales|layer2|greeting" {
			found++
			observedSum = sum
			if count != 1 {
				t.Errorf("expected count=1, got %d", count)
			}
		}
	})
	if found == 0 {
		t.Error("expected at least 1 sample with label 'ai_sales|layer2|greeting'")
	}
	if observedSum < 1.0 {
		t.Errorf("expected sum >= 1.0, got %f", observedSum)
	}
}

func TestRecordAIAgentLCPTime(t *testing.T) {
	RecordAIAgentLCPTime("smart_cs", "websocket", 0.123)

	GlobalMetrics.AIAgentLCPTime.Range(func(labels string, sum float64, count uint64) {
		if labels == "smart_cs|websocket" && count == 0 {
			t.Error("expected count > 0")
		}
	})
}

func TestRecordAIAgentLayerDecision(t *testing.T) {
	RecordAIAgentLayerDecision("layer1", "faq_hit")
	RecordAIAgentLayerDecision("layer2", "fallback")

	found := 0
	GlobalMetrics.AIAgentLayerDecision.Range(func(labels string, count uint64) {
		if labels == "layer1|faq_hit" || labels == "layer2|fallback" {
			found++
		}
	})
	if found < 2 {
		t.Errorf("expected at least 2 entries, got %d", found)
	}
}

func TestRecordAIAgentLLMCall(t *testing.T) {
	RecordAIAgentLLMCall("sop_reply", "local-7b-q5", "success")
	RecordAIAgentLLMCall("sop_reply", "local-3b-q4", "fallback")

	found := 0
	GlobalMetrics.AIAgentLLMCall.Range(func(labels string, count uint64) {
		if labels == "sop_reply|local-7b-q5|success" || labels == "sop_reply|local-3b-q4|fallback" {
			found++
		}
	})
	if found < 2 {
		t.Errorf("expected at least 2 entries, got %d", found)
	}
}

func TestRecordAIAgentFallback(t *testing.T) {
	RecordAIAgentFallback("local-7b-q5", "local-3b-q4", "timeout")
	RecordAIAgentFallback("local-3b-q4", "cache", "error")

	found := 0
	GlobalMetrics.AIAgentFallback.Range(func(labels string, count uint64) {
		if labels == "local-7b-q5|local-3b-q4|timeout" || labels == "local-3b-q4|cache|error" {
			found++
		}
	})
	if found < 2 {
		t.Errorf("expected at least 2 entries, got %d", found)
	}
}

func TestAIAgentMetrics_AllInitialized(t *testing.T) {
	// 验证 5 个 AI agent 指标都已初始化 (非 nil)
	if GlobalMetrics.AIAgentWallTime == nil {
		t.Error("AIAgentWallTime not initialized")
	}
	if GlobalMetrics.AIAgentLCPTime == nil {
		t.Error("AIAgentLCPTime not initialized")
	}
	if GlobalMetrics.AIAgentLayerDecision == nil {
		t.Error("AIAgentLayerDecision not initialized")
	}
	if GlobalMetrics.AIAgentLLMCall == nil {
		t.Error("AIAgentLLMCall not initialized")
	}
	if GlobalMetrics.AIAgentFallback == nil {
		t.Error("AIAgentFallback not initialized")
	}
}
