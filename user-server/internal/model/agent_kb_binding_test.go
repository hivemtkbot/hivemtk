package model

import (
	"testing"
	"time"
)

// TestAgentKBBinding_Fields 验证字段映射 + 角色/类型常量
func TestAgentKBBinding_Fields(t *testing.T) {
	now := time.Now()
	enabled := true
	b := &AgentKBBinding{
		ID:        1,
		AgentID:   100,
		KBID:      5,
		KBType:    KnowledgeBaseTypeFAQ,
		Role:      AgentKBBindingRolePrimary,
		Priority:  10,
		Enabled:   &enabled,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if b.AgentID != 100 {
		t.Errorf("AgentID mismatch: got %d, want 100", b.AgentID)
	}
	if b.KBID != 5 {
		t.Errorf("KBID mismatch: got %d, want 5", b.KBID)
	}
	if b.KBType != KnowledgeBaseTypeFAQ {
		t.Errorf("KBType mismatch: got %q, want %q", b.KBType, KnowledgeBaseTypeFAQ)
	}
	if b.Role != AgentKBBindingRolePrimary {
		t.Errorf("Role mismatch: got %q", b.Role)
	}
	if b.Priority != 10 {
		t.Errorf("Priority mismatch: got %d", b.Priority)
	}
	if b.Enabled == nil || !*b.Enabled {
		t.Errorf("Enabled should be true")
	}
}

// TestAgentKBBinding_TableName 验证表名
func TestAgentKBBinding_TableName(t *testing.T) {
	b := AgentKBBinding{}
	if got := b.TableName(); got != "agent_kb_bindings" {
		t.Errorf("TableName mismatch: got %q, want agent_kb_bindings", got)
	}
}

// TestAgentKBBinding_RoleConstants 验证 Role 常量
func TestAgentKBBinding_RoleConstants(t *testing.T) {
	if AgentKBBindingRolePrimary != "primary" {
		t.Errorf("Primary should be 'primary'")
	}
	if AgentKBBindingRoleReference != "reference" {
		t.Errorf("Reference should be 'reference'")
	}
}

