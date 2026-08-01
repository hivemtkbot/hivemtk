// Package service 知识库子域 —— 导入日志
//
// 导入行为审计日志写入函数 logImport。
// 单一职责：把"谁、什么时间、导入什么、结果如何"集中记录到 knowledge_import_logs。
package service

import (
	"context"

	"marketing/internal/aiagent/knowledge/model"
)

// logImport 记录导入日志
//
// 写入 knowledge_import_logs 表，承载：
//   - product_id（numeric 哈希）
//   - document_id（成功时非空，失败时为 nil）
//   - source_type/batch_no/operator/ip/user_agent
//   - status: "success" | "failed"
//   - duration_ms/error_detail
func (s *KnowledgeService) logImport(ctx context.Context, req *ImportRequest, docID uint64, status string, durationMs int, errMsg string) error {
	var docIDPtr *uint64
	if docID > 0 {
		docIDPtr = &docID
	}
	productNumericID := req.ProductID
	log := &model.KnowledgeImportLog{
		ProductID:   productNumericID,
		DocumentID:  docIDPtr,
		SourceType:  string(req.SourceType),
		BatchNo:     req.BatchNo,
		Status:      status,
		Operator:    req.Operator,
		IP:          req.IP,
		UserAgent:   req.UserAgent,
		DurationMs:  durationMs,
		ErrorDetail: errMsg,
	}
	return s.importLogRepo.Create(ctx, log)
}
