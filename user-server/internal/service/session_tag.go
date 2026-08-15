package service

import (
	"context"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)


// SessionTagService 会话标签服务
type SessionTagService struct {
	tagRepo *repository.SessionTagRepository
}

// NewSessionTagService 创建会话标签服务实例
func NewSessionTagService() *SessionTagService {
	return &SessionTagService{
		tagRepo: repository.NewSessionTagRepository(),
	}
}

// CreateTagRequest 创建标签请求
type CreateTagRequest struct {
	Name        string `json:"name" binding:"required"`
	Code        string `json:"code" binding:"required"`
	Group       string `json:"group"`
	Color       string `json:"color"`
	Description string `json:"description"`
	SortOrder   int    `json:"sort_order"`
}

// CreateTag 创建标签
func (s *SessionTagService) CreateTag(ctx context.Context, req *CreateTagRequest) (*model.SessionTag, error) {
	tag := &model.SessionTag{
		Name:        req.Name,
		Code:        req.Code,
		Group:       req.Group,
		Color:       req.Color,
		Description: req.Description,
		SortOrder:   req.SortOrder,
	}
	if tag.Color == "" {
		tag.Color = "#1890ff"
	}

	if err := s.tagRepo.Create(ctx, tag); err != nil {
		return nil, err
	}

	return tag, nil
}

// UpdateTag 更新标签
func (s *SessionTagService) UpdateTag(ctx context.Context, id uint, req *CreateTagRequest) (*model.SessionTag, error) {
	tag, err := s.tagRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	tag.Name = req.Name
	tag.Code = req.Code
	tag.Group = req.Group
	tag.Color = req.Color
	tag.Description = req.Description
	tag.SortOrder = req.SortOrder

	if err := s.tagRepo.Update(ctx, tag); err != nil {
		return nil, err
	}

	return tag, nil
}

// DeleteTag 删除标签
func (s *SessionTagService) DeleteTag(ctx context.Context, id uint) error {
	tag, err := s.tagRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	_ = tag

	return s.tagRepo.Delete(ctx, id)
}

// GetTags 获取标签列表
func (s *SessionTagService) GetTags(ctx context.Context) ([]*model.SessionTag, error) {
	return s.tagRepo.GetByMerchant(ctx)
}

