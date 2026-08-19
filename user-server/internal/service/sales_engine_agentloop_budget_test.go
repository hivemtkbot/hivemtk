package service

import (
	"testing"
	"time"
)

// TestSetAgentLoopMaxTotalTokens_NoOpOnZero 验证 ≤0 时保持默认 50000
func TestSetAgentLoopMaxTotalTokens_NoOpOnZero(t *testing.T) {
	orig := agentLoopMaxTotalTokens
	t.Cleanup(func() { agentLoopMaxTotalTokens = orig })

	SetAgentLoopMaxTotalTokens(0)
	if agentLoopMaxTotalTokens != orig {
		t.Errorf("0 should be no-op, got %d", orig)
	}
	SetAgentLoopMaxTotalTokens(-10)
	if agentLoopMaxTotalTokens != orig {
		t.Errorf("negative should be no-op, got %d", orig)
	}
}

// TestSetAgentLoopMaxTotalTokens_Applies 验证正值生效
func TestSetAgentLoopMaxTotalTokens_Applies(t *testing.T) {
	orig := agentLoopMaxTotalTokens
	t.Cleanup(func() { agentLoopMaxTotalTokens = orig })

	SetAgentLoopMaxTotalTokens(80000)
	if agentLoopMaxTotalTokens != 80000 {
		t.Errorf("expected 80000, got %d", agentLoopMaxTotalTokens)
	}
}

// TestSetAgentLoopMaxPerIterTimeout_NoOpOnZero 验证 ≤0 保持默认 60s
func TestSetAgentLoopMaxPerIterTimeout_NoOpOnZero(t *testing.T) {
	orig := agentLoopMaxPerIterTimeout
	t.Cleanup(func() { agentLoopMaxPerIterTimeout = orig })

	SetAgentLoopMaxPerIterTimeout(0)
	if agentLoopMaxPerIterTimeout != orig {
		t.Errorf("0 should be no-op, got %s", orig)
	}
	SetAgentLoopMaxPerIterTimeout(-5)
	if agentLoopMaxPerIterTimeout != orig {
		t.Errorf("negative should be no-op, got %s", orig)
	}
}

// TestSetAgentLoopMaxPerIterTimeout_Applies 验证正值生效
func TestSetAgentLoopMaxPerIterTimeout_Applies(t *testing.T) {
	orig := agentLoopMaxPerIterTimeout
	t.Cleanup(func() { agentLoopMaxPerIterTimeout = orig })

	SetAgentLoopMaxPerIterTimeout(45)
	if agentLoopMaxPerIterTimeout != 45*time.Second {
		t.Errorf("expected 45s, got %s", agentLoopMaxPerIterTimeout)
	}
}

// TestAgentLoopBudgetDefaults 验证默认值符合业界共识
func TestAgentLoopBudgetDefaults(t *testing.T) {
	if agentLoopMaxTotalTokens != 50000 {
		t.Errorf("default agentLoopMaxTotalTokens = %d, want 50000", agentLoopMaxTotalTokens)
	}
	if agentLoopMaxPerIterTimeout != 60*time.Second {
		t.Errorf("default agentLoopMaxPerIterTimeout = %s, want 60s", agentLoopMaxPerIterTimeout)
	}
	if agentLoopTotalTimeout != 180*time.Second {
		t.Errorf("default agentLoopTotalTimeout = %s, want 180s", agentLoopTotalTimeout)
	}
	// 业界硬约束：单次超时 < 总超时
	if agentLoopMaxPerIterTimeout >= agentLoopTotalTimeout {
		t.Errorf("per-iter timeout (%s) must be < total timeout (%s) to leave budget for tool execution",
			agentLoopMaxPerIterTimeout, agentLoopTotalTimeout)
	}
}
