package dto

import "time"

// Message 消息
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// ShortTermMemory 短期记忆
type ShortTermMemory struct {
	Messages []Message `json:"messages"`
}
