package service

import (
	"context"

	"errors"

	"strings"

	"time"

	"hivemtk-user/internal/model"
)

// upsertInboxFromHub 写入收件箱会话
func (s *WebhookService) upsertInboxFromHub(ctx context.Context, hub *model.MessageHub, customerName string) {
	if s.inboxConvRepo == nil || hub == nil {
		return
	}
	conv, err := s.inboxConvRepo.FindByPlatformAccountCustomer(ctx, hub.Platform, hub.AccountID, hub.SenderID)
	if err == nil && conv != nil {

		_ = s.inboxConvRepo.UpdateLastMessage(ctx, conv.ID, hub.Content, hub.SentAt, 1)
		return
	}
	newConv := &model.InboxConversation{
		Platform:           hub.Platform,
		AccountID:          hub.AccountID,
		CustomerID:         hub.SenderID,
		CustomerName:       customerName,
		LastMessagePreview: hub.Content,
		LastMessageAt:      &hub.SentAt,
		UnreadCount:        1,
		Status:             "active",
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	_ = s.inboxConvRepo.Create(ctx, newConv)
}

func (s *WebhookService) dispatchToUnified(ctx context.Context, um *model.UnifiedMessage) error {
	s.ensureReposFromDB(ctx)
	if s.unifiedMsgRepo == nil {
		return errors.New("unified message repo nil")
	}

	if err := s.unifiedMsgRepo.Create(ctx, um); err != nil {

		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate") {
			return nil
		}
		return err
	}
	return nil
}
