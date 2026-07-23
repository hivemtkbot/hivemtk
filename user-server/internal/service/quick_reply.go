package service

import (
	"context"
	"marketing/internal/model"
	"marketing/internal/repository"
)

// ============================================================================
// 快捷回复服务（quick_reply.go）
// ----------------------------------------------------------------------------
// 从 customer_session.go 拆分（2026-07-22 方向C）。
// 职责：坐席侧快捷回复模板（按 category 分类）。
// 文档：docs/企业级架构优化/坐席实时聊天看板.md §二
// ============================================================================

// QuickReplyService 快捷回复服务
type QuickReplyService struct {
	replyRepo *repository.QuickReplyRepository
}

// NewQuickReplyService 创建快捷回复服务实例
func NewQuickReplyService() *QuickReplyService {
	return &QuickReplyService{
		replyRepo: repository.NewQuickReplyRepository(),
	}
}

// CreateReplyRequest 创建快捷回复请求
type CreateReplyRequest struct {
	Category  string `json:"category" binding:"required"`
	Title     string `json:"title" binding:"required"`
	Content   string `json:"content" binding:"required"`
	Channel   string `json:"channel"`
	SortOrder int    `json:"sort_order"`
	IsPublic  bool   `json:"is_public"`
}

// CreateReply 创建快捷回复
func (s *QuickReplyService) CreateReply(ctx context.Context, createdBy uint, req *CreateReplyRequest) (*model.QuickReply, error) {
	reply := &model.QuickReply{
		Category:  req.Category,
		Title:     req.Title,
		Content:   req.Content,
		Channel:   req.Channel,
		SortOrder: req.SortOrder,
		IsPublic:  req.IsPublic,
		CreatedBy: createdBy,
	}
	if !reply.IsPublic {
		reply.IsPublic = true // 默认公开
	}

	if err := s.replyRepo.Create(ctx, reply); err != nil {
		return nil, err
	}

	return reply, nil
}

// UpdateReply 更新快捷回复
func (s *QuickReplyService) UpdateReply(ctx context.Context, id uint, req *CreateReplyRequest) (*model.QuickReply, error) {
	reply, err := s.replyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	reply.Category = req.Category
	reply.Title = req.Title
	reply.Content = req.Content
	reply.Channel = req.Channel
	reply.SortOrder = req.SortOrder
	reply.IsPublic = req.IsPublic

	if err := s.replyRepo.Update(ctx, reply); err != nil {
		return nil, err
	}

	return reply, nil
}

// DeleteReply 删除快捷回复
func (s *QuickReplyService) DeleteReply(ctx context.Context, id uint) error {
	reply, err := s.replyRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	_ = reply

	return s.replyRepo.Delete(ctx, id)
}

// GetReplies 获取快捷回复列表
func (s *QuickReplyService) GetReplies(ctx context.Context, category string) ([]*model.QuickReply, error) {
	return s.replyRepo.GetByMerchant(ctx, category)
}

// GetCategories 获取快捷回复分类
func (s *QuickReplyService) GetCategories(ctx context.Context) ([]string, error) {
	return s.replyRepo.GetCategories(ctx)
}
