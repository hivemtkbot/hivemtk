package agent_runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"hivemtk-user/internal/event"
)

// PublishKnowledgeDocumentChange 发布知识库文档变更事件
//
// 参数:
//   - workspaceID: 知识库工作区 ID(对应 RagProduct.ID)
//   - documentID:  文档 ID
//   - changeType:  create / update / delete
//   - content:     文档内容(用于计算 hash,nil 时只发事件不计算)
//   - operatorID:  操作人 ID
//   - traceID:     追踪 ID
//
// 返回:生成的 TraceID
func PublishKnowledgeDocumentChange(workspaceID string, documentID uint, changeType, content string, operatorID uint, traceID string) string {
	if traceID == "" {
		traceID = "rag_" + time.Now().Format("20060102150405.000000")
	}

	payload := event.KnowledgeDocumentChangePayload{
		WorkspaceID: workspaceID,
		DocumentID:  documentID,
		ChangeType:  changeType,
		OperatorID:  operatorID,
		TraceID:     traceID,
	}

	if content != "" {
		h := sha256.Sum256([]byte(content))
		payload.ContentHash = hex.EncodeToString(h[:])
	}

	event.Publish(event.TopicKnowledgeDocumentChanged, payload)
	return traceID
}

// PublishKnowledgeDocumentCreate 快捷方法:create
func PublishKnowledgeDocumentCreate(workspaceID string, documentID uint, content string, operatorID uint) string {
	return PublishKnowledgeDocumentChange(workspaceID, documentID, "create", content, operatorID, "")
}

// PublishKnowledgeDocumentUpdate 快捷方法:update
func PublishKnowledgeDocumentUpdate(workspaceID string, documentID uint, content string, operatorID uint) string {
	return PublishKnowledgeDocumentChange(workspaceID, documentID, "update", content, operatorID, "")
}

// PublishKnowledgeDocumentDelete 快捷方法:delete
func PublishKnowledgeDocumentDelete(workspaceID string, documentID uint, operatorID uint) string {
	return PublishKnowledgeDocumentChange(workspaceID, documentID, "delete", "", operatorID, "")
}
