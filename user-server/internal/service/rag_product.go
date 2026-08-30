package service

import (
	"context"
	"errors"

	kbrepo "hivemtk-user/internal/aiagent/knowledge/repository"
	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

type RagProductService struct {
	db   *gorm.DB
	repo *kbrepo.RagConfigRepository
}

func NewRagProductService(db *gorm.DB) *RagProductService {
	return &RagProductService{db: db, repo: kbrepo.NewRagConfigRepository(db)}
}

func (s *RagProductService) List(ctx context.Context) ([]*model.RagProduct, error) {
	var products []*model.RagProduct
	if err := s.db.WithContext(ctx).Find(&products).Error; err != nil {
		return nil, err
	}
	return products, nil
}

func (s *RagProductService) Get(ctx context.Context, id string) (*model.RagProduct, error) {
	var p model.RagProduct
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("rag product not found")
		}
		return nil, err
	}
	return &p, nil
}

func (s *RagProductService) Create(ctx context.Context, p *model.RagProduct) error {
	if p.VectorTable == "" {
		p.VectorTable = "rag_vectors_" + p.ID
	}
	return s.db.WithContext(ctx).Create(p).Error
}

func (s *RagProductService) Update(ctx context.Context, p *model.RagProduct) error {
	return s.repo.UpdateRagProduct(ctx, p)
}

func (s *RagProductService) Delete(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Where("id = ?", id).Delete(&model.RagProduct{}).Error
}

func (s *RagProductService) Stats(ctx context.Context) (map[string]any, error) {
	var total int64
	var active int64
	s.db.WithContext(ctx).Model(&model.RagProduct{}).Count(&total)
	s.db.WithContext(ctx).Model(&model.RagProduct{}).Where("is_active = ?", true).Count(&active)
	var totalDocs int64
	var totalChunks int64
	s.db.WithContext(ctx).Model(&model.RagProduct{}).Select("COALESCE(SUM(doc_count),0)").Scan(&totalDocs)
	s.db.WithContext(ctx).Model(&model.RagProduct{}).Select("COALESCE(SUM(chunk_count),0)").Scan(&totalChunks)
	return map[string]any{
		"total":        total,
		"active":       active,
		"inactive":     total - active,
		"total_docs":   totalDocs,
		"total_chunks": totalChunks,
	}, nil
}
