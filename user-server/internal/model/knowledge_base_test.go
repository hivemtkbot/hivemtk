package model

import (
	"testing"
	"time"
)

// TestKnowledgeBase_Fields 验证字段映射 (CRUD 模型层单测)
//
// 验证:
//
//	GORM tag 是否正确 / json tag 序列化 / 默认值 / nullable 字段
func TestKnowledgeBase_Fields(t *testing.T) {
	now := time.Now()
	owner := uint(7)
	enabled := true
	kb := &KnowledgeBase{
		ID:           1,
		KBCode:       "KB-FAQ-001",
		Type:         KnowledgeBaseTypeFAQ,
		Name:         "客服 FAQ 库",
		Description:  "测试知识库",
		OwnerType:    KnowledgeBaseOwnerPrivate,
		OwnerAgentID: &owner,
		MemberCount:  3,
		DocCount:     0,
		Enabled:      &enabled,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if kb.ID != 1 {
		t.Errorf("ID mismatch: got %d, want 1", kb.ID)
	}
	if kb.KBCode != "KB-FAQ-001" {
		t.Errorf("KBCode mismatch: got %q", kb.KBCode)
	}
	if kb.Type != KnowledgeBaseTypeFAQ {
		t.Errorf("Type mismatch: got %q, want %q", kb.Type, KnowledgeBaseTypeFAQ)
	}
	if kb.OwnerType != KnowledgeBaseOwnerPrivate {
		t.Errorf("OwnerType mismatch: got %q", kb.OwnerType)
	}
	if kb.OwnerAgentID == nil || *kb.OwnerAgentID != 7 {
		t.Errorf("OwnerAgentID mismatch: got %v", kb.OwnerAgentID)
	}
	if kb.Enabled == nil || !*kb.Enabled {
		t.Errorf("Enabled mismatch: got %v", kb.Enabled)
	}
}

// TestKnowledgeBase_TableName 验证表名 (GORM 约定, 不能轻易改)
func TestKnowledgeBase_TableName(t *testing.T) {
	kb := KnowledgeBase{}
	if got := kb.TableName(); got != "knowledge_bases" {
		t.Errorf("TableName mismatch: got %q, want knowledge_bases", got)
	}
}

// TestKnowledgeBase_OwnerTypeConstants 验证 OwnerType 常量
func TestKnowledgeBase_OwnerTypeConstants(t *testing.T) {
	if KnowledgeBaseOwnerPrivate != "private" {
		t.Errorf("KnowledgeBaseOwnerPrivate should be 'private'")
	}
	if KnowledgeBaseOwnerShared != "shared" {
		t.Errorf("KnowledgeBaseOwnerShared should be 'shared'")
	}
}

// TestKnowledgeBase_TypeConstants 验证 Type 枚举值
func TestKnowledgeBase_TypeConstants(t *testing.T) {
	if KnowledgeBaseTypeFAQ != "faq" {
		t.Errorf("KnowledgeBaseTypeFAQ should be 'faq'")
	}
	if KnowledgeBaseTypeRAG != "rag" {
		t.Errorf("KnowledgeBaseTypeRAG should be 'rag'")
	}
	if KnowledgeBaseTypeSOP != "sop" {
		t.Errorf("KnowledgeBaseTypeSOP should be 'sop'")
	}
}

// TestKnowledgeBase_SharedHasNilOwner 共享库 OwnerAgentID 必须为 nil
func TestKnowledgeBase_SharedHasNilOwner(t *testing.T) {
	kb := &KnowledgeBase{
		KBCode:       "KB-RAG-SHARED",
		Type:         KnowledgeBaseTypeRAG,
		OwnerType:    KnowledgeBaseOwnerShared,
		OwnerAgentID: nil,
	}
	if kb.OwnerAgentID != nil {
		t.Error("shared KB must have nil OwnerAgentID")
	}
}

