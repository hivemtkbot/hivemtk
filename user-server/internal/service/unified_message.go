package service

import (
	"context"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// UnifiedMessageService 统一消息服务（仅保留消息查询视图，自动回复决策已移除）
type UnifiedMessageService struct {
	messageRepo repository.UnifiedMessageRepository
}

// NewUnifiedMessageService 创建统一消息服务
func NewUnifiedMessageService() *UnifiedMessageService {
	return &UnifiedMessageService{
		messageRepo: repository.NewUnifiedMessageRepository(),
	}
}

// GetMessages 获取消息列表（按平台查询，自动回复已移除，仅保留消息视图）
func (s *UnifiedMessageService) GetMessages(ctx context.Context, platform string, page, pageSize int) ([]*model.UnifiedMessage, int, error) {
	messages, total, err := s.messageRepo.GetByMerchant(ctx, model.Platform(platform), page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	return messages, int(total), nil
}

// GetMessageByID 获取消息详情
func (s *UnifiedMessageService) GetMessageByID(ctx context.Context, id uint) (*model.UnifiedMessage, error) {
	return s.messageRepo.GetByID(ctx, id)
}

