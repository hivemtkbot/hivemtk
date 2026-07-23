// Package service 知识库子域 —— 辅助工具函数
//
// 2026-07-23 五层架构治理（三轮）：从 knowledge_base.go 拆出
// 通用辅助函数。单一职责：与 KnowledgeBase 业务无关的可复用工具。
package service

import (
	"os"
)

// getFileSize 获取文件大小
func getFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
