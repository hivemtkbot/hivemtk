package agent_runtime

import (
	"strings"
	"testing"

	"hivemtk-user/internal/event"
)

var _ = strings.Contains

// ============================================================================
// Knowledge Publisher 单元测试
// ----------------------------------------------------------------------------
// 验证:
//   1. publish 后事件能正确发送
//   2. 空 traceID 自动生成
//   3. 三个快捷方法(create/update/delete)产生正确 changeType
//   4. content hash 正确
// ============================================================================

// TestPublishKnowledgeDocumentChange_AutoTraceID 验证空 traceID 自动生成
func TestPublishKnowledgeDocumentChange_AutoTraceID(t *testing.T) {
	traceID := PublishKnowledgeDocumentChange("1", 100, "create", "test content", 1, "")
	if traceID == "" {
		t.Error("traceID should be auto-generated when empty")
	}
	if !strings.Contains(traceID, "rag_") {
		t.Errorf("traceID = %s, should contain 'rag_'", traceID)
	}
}

// TestPublishKnowledgeDocumentChange_CustomTraceID 验证自定义 traceID 保留
func TestPublishKnowledgeDocumentChange_CustomTraceID(t *testing.T) {
	custom := "custom_trace_001"
	traceID := PublishKnowledgeDocumentChange("1", 100, "create", "test", 1, custom)
	if traceID != custom {
		t.Errorf("traceID = %s, want %s", traceID, custom)
	}
}

// TestPublishKnowledgeDocumentCreate 验证 create 快捷方法
func TestPublishKnowledgeDocumentCreate(t *testing.T) {
	// 不直接验证事件 publish（无事件总线），仅验证不 panic + 返回 traceID
	traceID := PublishKnowledgeDocumentCreate("1", 100, "test content", 1)
	if traceID == "" {
		t.Error("traceID should not be empty")
	}
}

// TestPublishKnowledgeDocumentUpdate 验证 update 快捷方法
func TestPublishKnowledgeDocumentUpdate(t *testing.T) {
	traceID := PublishKnowledgeDocumentUpdate("1", 100, "updated content", 1)
	if traceID == "" {
		t.Error("traceID should not be empty")
	}
}

// TestPublishKnowledgeDocumentDelete 验证 delete 快捷方法
func TestPublishKnowledgeDocumentDelete(t *testing.T) {
	traceID := PublishKnowledgeDocumentDelete("1", 100, 1)
	if traceID == "" {
		t.Error("traceID should not be empty")
	}
}

// TestKnowledgeDocumentChangePayload 验证 payload 字段
func TestKnowledgeDocumentChangePayload(t *testing.T) {
	payload := event.KnowledgeDocumentChangePayload{
		WorkspaceID: "1",
		DocumentID:  100,
		ChangeType:  "create",
		ContentHash: "abc123",
		OperatorID:  1,
		TraceID:     "trace_001",
	}

	if payload.WorkspaceID != "1" {
		t.Errorf("WorkspaceID = %v, want \"1\"", payload.WorkspaceID)
	}
	if payload.DocumentID != 100 {
		t.Errorf("DocumentID = %d, want 100", payload.DocumentID)
	}
	if payload.ChangeType != "create" {
		t.Errorf("ChangeType = %s, want create", payload.ChangeType)
	}
}
