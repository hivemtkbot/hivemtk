package dto

import "marketing/internal/model"

// RecoveryEnqueueRequest 入队请求
type RecoveryEnqueueRequest struct {
	CustomerID string `json:"customer_id" binding:"required"`
	UnifiedID  string `json:"unified_id"`
	Account    string `json:"account"`
	Reason     string `json:"reason"`
	Strategy   string `json:"strategy"`
	Priority   int    `json:"priority"`
}

// RecoveryMarkAttemptRequest 触达尝试请求
type RecoveryMarkAttemptRequest struct {
	Channel   string `json:"channel" binding:"required"`
	Result    string `json:"result"`
	Stage     string `json:"stage" binding:"required"` // succeed/failed/running
	NextDelay int    `json:"next_delay_seconds"`       // 0 = 不重试
}

// RecoveryMarkRecoveredRequest 标记挽回成功
type RecoveryMarkRecoveredRequest struct {
	RecoveryValue int64 `json:"recovery_value"`
}

// RecoveryQueueResponse 单条响应
type RecoveryQueueResponse struct {
	ID            uint64  `json:"id"`
	CustomerID    string  `json:"customer_id"`
	UnifiedID     string  `json:"unified_id"`
	Account       string  `json:"account"`
	Reason        string  `json:"reason"`
	Strategy      string  `json:"strategy"`
	Priority      int     `json:"priority"`
	Stage         string  `json:"stage"`
	Attempts      int     `json:"attempts"`
	MaxAttempts   int     `json:"max_attempts"`
	LastChannel   string  `json:"last_channel"`
	LastResult    string  `json:"last_result"`
	RecoveryValue int64   `json:"recovery_value"`
	MetaJSON      string  `json:"meta_json"`
	LastAttemptAt *string `json:"last_attempt_at"`
	NextAttemptAt *string `json:"next_attempt_at"`
	RecoveredAt   *string `json:"recovered_at"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

// FromModel RecoveryQueue → Response
func (r *RecoveryQueueResponse) FromModel(item *model.RecoveryQueue) *RecoveryQueueResponse {
	if item == nil {
		return nil
	}
	resp := &RecoveryQueueResponse{
		ID:            item.ID,
		CustomerID:    item.CustomerID,
		UnifiedID:     item.UnifiedID,
		Account:       item.Account,
		Reason:        item.Reason,
		Strategy:      item.Strategy,
		Priority:      item.Priority,
		Stage:         item.Stage,
		Attempts:      item.Attempts,
		MaxAttempts:   item.MaxAttempts,
		LastChannel:   item.LastChannel,
		LastResult:    item.LastResult,
		RecoveryValue: item.RecoveryValue,
		MetaJSON:      item.MetaJSON,
		CreatedAt:     item.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     item.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	if item.LastAttemptAt != nil {
		s := item.LastAttemptAt.UTC().Format("2006-01-02T15:04:05Z")
		resp.LastAttemptAt = &s
	}
	if item.NextAttemptAt != nil {
		s := item.NextAttemptAt.UTC().Format("2006-01-02T15:04:05Z")
		resp.NextAttemptAt = &s
	}
	if item.RecoveredAt != nil {
		s := item.RecoveredAt.UTC().Format("2006-01-02T15:04:05Z")
		resp.RecoveredAt = &s
	}
	return resp
}

// RecoveryQueueListResponse 列表响应
type RecoveryQueueListResponse struct {
	List     []*RecoveryQueueResponse `json:"list"`
	Total    int64                    `json:"total"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"page_size"`
}

// RecoveryDistributionResponse 阶段分布
type RecoveryDistributionResponse struct {
	Distribution map[string]int64 `json:"distribution"`
	Total        int64            `json:"total"`
}
