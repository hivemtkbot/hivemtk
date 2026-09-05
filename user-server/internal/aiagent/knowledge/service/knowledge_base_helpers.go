// Package service 知识库子域 —— 辅助工具函数
//
// 通用辅助函数。单一职责：与 KnowledgeBase 业务无关的可复用工具。
package service

import (
	"os"
)

func getFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
