// Package service 知识库子域 —— 通用辅助函数
//
// 与 service 业务无关的"通用工具"(哈希、mime 推断)。
// 单一职责:放可被 service 多个子域复用的最小化工具函数。
//
// 本文件仅保留与文件/扩展名相关的服务级工具。
package service

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
