package service

import (
	"marketing/internal/model"
	"marketing/internal/repository"
)

type MessageService struct {
	repo repository.MessageRepository
}

func NewMessageService() *MessageService {
	return &MessageService{repo: repository.NewMessageRepository()}
}

func (s *MessageService) Register(message model.Message) (*model.Message, error) {
	if err := s.repo.Create(&message); err != nil {
		return nil, err
	}
	return &message, nil
}

func (s *MessageService) GetMessage(id uint) (*model.Message, error) {
	return s.repo.GetByID(id)
}

func (s *MessageService) GetMessageList(page int, limit int) ([]*model.Message, int64, error) {
	return s.repo.GetMessageList(page, limit)
}

func (s *MessageService) DeleteMessage(id string) error {
	return s.repo.Delete(id)
}

func (s *MessageService) InitMessage(accountID string, userID string, tgID int64, text string) (string, error) {
	// 直接使用string类型的ID
	message := model.Message{
		AccountID: accountID,
		UserID:    userID,
		TgID:      tgID,
		Text:      text,
	}
	if err := s.repo.Create(&message); err != nil {
		return "", err
	}
	return message.ID, nil
}
