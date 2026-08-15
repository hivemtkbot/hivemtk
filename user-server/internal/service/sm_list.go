package service

import (
	"context"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

type SmlistService struct {
	repo repository.SmlistRepository
}

func NewSmlistService(repo ...repository.SmlistRepository) *SmlistService {
	if len(repo) > 0 {
		return &SmlistService{repo: repo[0]}
	}
	return &SmlistService{repo: repository.NewSmlistRepository()}
}

func (s *SmlistService) Register(ctx context.Context, smlist model.Smlist) (*model.Smlist, error) {
	if err := s.repo.Create(ctx, &smlist); err != nil {
		return nil, err
	}
	return &smlist, nil
}

func (s *SmlistService) GetSmlist(ctx context.Context, id string) (*model.Smlist, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *SmlistService) GetSmlistList(ctx context.Context, page int, limit int) ([]*model.Smlist, int64, error) {
	return s.repo.GetSmlistList(ctx, page, limit)
}

func (s *SmlistService) GetSmlistAllList(ctx context.Context) ([]*model.Smlist, int64, error) {
	return s.repo.GetSmlistAllList(ctx)
}

func (s *SmlistService) DeleteSmlist(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}
func (s *SmlistService) GetRecentSmlistList(ctx context.Context) ([]*model.Smlist, error) {
	return s.repo.GetRecentSmlistList(ctx)
}

