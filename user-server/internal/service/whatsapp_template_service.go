package service

import (
	"errors"
	"fmt"
	"time"

	"marketing/internal/model"
	"marketing/internal/repository"

	"gorm.io/gorm"
	"context"
)

type WhatsAppTemplateService struct {
	db	*gorm.DB	// 保留以维持签名兼容（controller 直接传入）
	repo	*repository.WhatsappTemplateRepository
}

func NewWhatsAppTemplateService(db *gorm.DB) *WhatsAppTemplateService {
	var repo *repository.WhatsappTemplateRepository
	if db != nil {
		repo = repository.NewWhatsappTemplateRepositoryWithDB(db)
	}
	return &WhatsAppTemplateService{db: db, repo: repo}
}

func (ts *WhatsAppTemplateService) CreateTemplate(ctx context.Context, template *model.WhatsappMessageTemplate) (*model.WhatsappMessageTemplate, error) {
	if template.ID == "" {
		template.ID = generateTemplateID()
	}
	now := time.Now()
	template.CreatedAt = now
	template.UpdatedAt = now

	if err := ts.repo.Create(template); err != nil {
		return nil, err
	}
	return template, nil
}

func (ts *WhatsAppTemplateService) UpdateTemplate(ctx context.Context, template *model.WhatsappMessageTemplate) (*model.WhatsappMessageTemplate, error) {
	template.UpdatedAt = time.Now()
	if err := ts.repo.Save(template); err != nil {
		return nil, err
	}
	return template, nil
}

func (ts *WhatsAppTemplateService) GetTemplate(ctx context.Context, templateID string) (*model.WhatsappMessageTemplate, error) {
	template, err := ts.repo.GetByID(templateID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("模板不存在")
		}
		return nil, err
	}
	return template, nil
}

func (ts *WhatsAppTemplateService) GetTemplates(ctx context.Context, category string, isActive *bool) ([]*model.WhatsappMessageTemplate, error) {
	return ts.repo.ListByFilters(category, isActive)
}

func (ts *WhatsAppTemplateService) DeleteTemplate(ctx context.Context, templateID string) error {
	rowsAffected, err := ts.repo.DeleteByID(templateID)
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("模板不存在")
	}
	return nil
}

func generateTemplateID() string {
	return fmt.Sprintf("tmpl_%d", time.Now().UnixNano())
}
