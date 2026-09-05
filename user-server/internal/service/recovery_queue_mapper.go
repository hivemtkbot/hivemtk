package service

import (
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
)

func FromRecoveryQueueModel(item *model.RecoveryQueue) *dto.RecoveryQueueResponse {
	if item == nil {
		return nil
	}
	resp := &dto.RecoveryQueueResponse{
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
