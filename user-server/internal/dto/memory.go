package dto

import "time"

// memory.go 销冠域 - 对话记忆 DTO
//
// 从 service 包迁入（DTO 层补全）：
//   - Message：对话消息，由 DialogueMemoryService.AppendMessage 接收、GetShortTermMemory 返回
//   - ShortTermMemory：短期记忆聚合，承载最近 N 轮消息

// Message 消息
type Message struct {
	Role      string    `json:"role"` // user / ai / agent
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// ShortTermMemory 短期记忆
type ShortTermMemory struct {
	Messages []Message `json:"messages"`
}
