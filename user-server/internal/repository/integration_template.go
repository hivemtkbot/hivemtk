package repository

import (
	"errors"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

// IntegrationTemplateRepository 第三方对接模板仓库
type IntegrationTemplateRepository interface {
	Create(t *model.IntegrationTemplate) error
	Update(t *model.IntegrationTemplate) error
	Delete(id uint64) error
	GetByID(id uint64) (*model.IntegrationTemplate, error)
	GetByCode(code string) (*model.IntegrationTemplate, error)
	List(platform, category string, enabled *bool, page, pageSize int) ([]*model.IntegrationTemplate, int64, error)
	ListBuiltIn() ([]*model.IntegrationTemplate, error)
	UpsertBuiltIn(t *model.IntegrationTemplate) error // 用于种子化预置模板
}

type integrationTemplateRepo struct {
	db *gorm.DB
}

func NewIntegrationTemplateRepository() IntegrationTemplateRepository {
	return &integrationTemplateRepo{db: _db.GetDB()}
}

func NewIntegrationTemplateRepositoryWithDB(db *gorm.DB) IntegrationTemplateRepository {
	return &integrationTemplateRepo{db: db}
}

func (r *integrationTemplateRepo) Create(t *model.IntegrationTemplate) error {
	if t.Code == "" {
		return errors.New("模板编码不能为空")
	}
	// 唯一性检查
	if existing, _ := r.GetByCode(t.Code); existing != nil {
		return errors.New("模板编码已存在")
	}
	return r.db.Create(t).Error
}

func (r *integrationTemplateRepo) Update(t *model.IntegrationTemplate) error {
	if t.ID == 0 {
		return errors.New("id 不能为空")
	}
	return r.db.Save(t).Error
}

func (r *integrationTemplateRepo) Delete(id uint64) error {
	// 内置模板不允许删除（业务保护）
	var t model.IntegrationTemplate
	if err := r.db.First(&t, id).Error; err != nil {
		return err
	}
	if t.BuiltIn {
		return errors.New("系统预置模板不允许删除")
	}
	return r.db.Delete(&model.IntegrationTemplate{}, id).Error
}

func (r *integrationTemplateRepo) GetByID(id uint64) (*model.IntegrationTemplate, error) {
	var t model.IntegrationTemplate
	if err := r.db.First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *integrationTemplateRepo) GetByCode(code string) (*model.IntegrationTemplate, error) {
	var t model.IntegrationTemplate
	if err := r.db.Where("code = ?", code).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *integrationTemplateRepo) List(platform, category string, enabled *bool, page, pageSize int) ([]*model.IntegrationTemplate, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	var items []*model.IntegrationTemplate
	var total int64
	q := r.db.Model(&model.IntegrationTemplate{})
	if platform != "" {
		q = q.Where("platform = ?", platform)
	}
	if category != "" {
		q = q.Where("category = ?", category)
	}
	if enabled != nil {
		q = q.Where("enabled = ?", *enabled)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("platform ASC, code ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *integrationTemplateRepo) ListBuiltIn() ([]*model.IntegrationTemplate, error) {
	var items []*model.IntegrationTemplate
	if err := r.db.Where("is_built_in = ?", true).Order("platform ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *integrationTemplateRepo) UpsertBuiltIn(t *model.IntegrationTemplate) error {
	if t.Code == "" {
		return errors.New("code 不能为空")
	}
	var existing model.IntegrationTemplate
	err := r.db.Where("code = ?", t.Code).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(t).Error
	}
	// 内置模板：仅更新可变字段（auth_config / field_maps / endpoints / api_base / doc_url / remark / enabled）
	t.ID = existing.ID
	t.CreatedAt = existing.CreatedAt
	return r.db.Model(&model.IntegrationTemplate{}).Where("id = ?", existing.ID).Updates(map[string]any{
		"api_base":    t.APIBase,
		"auth_config": t.AuthConfig,
		"doc_url":     t.DocURL,
		"field_maps":  t.FieldMaps,
		"endpoints":   t.Endpoints,
		"enabled":     t.Enabled,
		"remark":      t.Remark,
	}).Error
}
