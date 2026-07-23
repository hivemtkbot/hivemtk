// Package service 知识库子域 —— 通用辅助函数
//
// 2026-07-23 五层架构治理(二轮):从 knowledge_service.go 拆出
// 与 service 业务无关的"通用工具"(哈希、mime 推断)。
// 单一职责:放可被 service 多个子域复用的最小化工具函数。
//
// 2026-07-23 五层架构治理(三轮):通用哈希已外迁至 internal/pkg/utils/strhash,
// 本文件仅保留与文件/扩展名相关的服务级工具(薄包装 hash 已上提)。
package service

import (
	"marketing/internal/pkg/utils/strhash"
)

// HashStringToInt64 字符串哈希到 int64(已迁移到 strhash.StringToInt64)
//
// UUID 字符串 → int64:知识库主键是 UUID 字符串,而 knowledge_chunks.product_id
// 是 INTEGER 字段,存储 HashStringToInt64(UUID) 映射值以兼容既有 schema。
func HashStringToInt64(s string) int64 {
	return strhash.StringToInt64(s)
}

// getMimeType 根据文件扩展名推断 MIME
func getMimeType(ext string) string {
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".docx", ".doc":
		return "application/msword"
	case ".txt":
		return "text/plain"
	case ".md":
		return "text/markdown"
	case ".html":
		return "text/html"
	case ".json":
		return "application/json"
	case ".csv":
		return "text/csv"
	default:
		return "application/octet-stream"
	}
}
