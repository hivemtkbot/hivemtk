package service

import (
	"marketing/internal/model"
	"marketing/internal/repository"
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

func (s *SmlistService) Register(smlist model.Smlist) (*model.Smlist, error) {
	if err := s.repo.Create(&smlist); err != nil {
		return nil, err
	}
	return &smlist, nil
}

func (s *SmlistService) GetSmlist(id string) (*model.Smlist, error) {
	return s.repo.GetByID(id)
}

func (s *SmlistService) GetSmlistList(page int, limit int) ([]*model.Smlist, int64, error) {
	return s.repo.GetSmlistList(page, limit)
}

func (s *SmlistService) GetSmlistAllList() ([]*model.Smlist, int64, error) {
	return s.repo.GetSmlistAllList()
}

func (s *SmlistService) DeleteSmlist(id string) error {
	return s.repo.Delete(id)
}
func (s *SmlistService) GetRecentSmlistList() ([]*model.Smlist, error) {
	return s.repo.GetRecentSmlistList()
}
