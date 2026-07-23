package service

import (
	"marketing/internal/model"
	"marketing/internal/repository"
	"context"
)

type MessageService struct {
	repo repository.MessageRepository
}

func NewMessageService() *MessageService {
	return &MessageService{repo: repository.NewMessageRepository()}
}

func (s *MessageService) Register(ctx context.Context, message model.Message) (*model.Message, error) {
	if err := s.repo.Create(ctx, &message); err != nil {
		return nil, err
	}
	return &message, nil
}

func (s *MessageService) GetMessage(ctx context.Context, id uint) (*model.Message, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *MessageService) GetMessageList(ctx context.Context, page int, limit int) ([]*model.Message, int64, error) {
	return s.repo.GetMessageList(ctx, page, limit)
}

func (s *MessageService) DeleteMessage(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *MessageService) InitMessage(ctx context.Context, accountID string, userID string, tgID int64, text string) (string, error) {
	// 直接使用string类型的ID
	message := model.Message{
		AccountID:	accountID,
		UserID:		userID,
		TgID:		tgID,
		Text:		text,
	}
	if err := s.repo.Create(ctx, &message); err != nil {
		return "", err
	}
	return message.ID, nil
}
