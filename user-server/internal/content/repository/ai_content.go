package repository

import (
	"marketing/internal/content/model"

	"gorm.io/gorm"
)

// AIGenerationRecordRepository AI生成记录仓库接口
type AIGenerationRecordRepository interface {
	Create(record *model.AIGenerationRecord) error
	GetByID(id uint) (*model.AIGenerationRecord, error)
	GetByMerchantAndUser(userID uint, page, pageSize int, filters map[string]any) ([]*model.AIGenerationRecord, int64, error)
	UpdateSaved(id uint, isSaved bool) error
	UpdateFavorite(id uint, isFavorite bool) error
	UpdateRating(id uint, rating int) error
	Delete(id uint) error
	CountByMerchantAndUser(userID uint) (int64, error)
}

type aiGenerationRecordRepo struct {
	db *gorm.DB
}

// NewAIGenerationRecordRepository 创建AI生成记录仓库实例
func NewAIGenerationRecordRepository(db *gorm.DB) AIGenerationRecordRepository {
	return &aiGenerationRecordRepo{db: db}
}

func (r *aiGenerationRecordRepo) Create(record *model.AIGenerationRecord) error {
	return r.db.Create(record).Error
}

func (r *aiGenerationRecordRepo) GetByID(id uint) (*model.AIGenerationRecord, error) {
	var record model.AIGenerationRecord
	err := r.db.First(&record, id).Error
	return &record, err
}

func (r *aiGenerationRecordRepo) GetByMerchantAndUser(userID uint, page, pageSize int, filters map[string]any) ([]*model.AIGenerationRecord, int64, error) {
	var records []*model.AIGenerationRecord
	var total int64

	query := r.db.Model(&model.AIGenerationRecord{}).Where("user_id = ?", userID)

	// 应用过滤条件
	if recordType, ok := filters["type"]; ok && recordType != "" {
		query = query.Where("type = ?", recordType)
	}
	if isSaved, ok := filters["is_saved"]; ok {
		query = query.Where("is_saved = ?", isSaved)
	}
	if isFavorite, ok := filters["is_favorite"]; ok {
		query = query.Where("is_favorite = ?", isFavorite)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&records).Error
	return records, total, err
}

func (r *aiGenerationRecordRepo) UpdateSaved(id uint, isSaved bool) error {
	return r.db.Model(&model.AIGenerationRecord{}).Where("id = ?", id).Update("is_saved", isSaved).Error
}

func (r *aiGenerationRecordRepo) UpdateFavorite(id uint, isFavorite bool) error {
	return r.db.Model(&model.AIGenerationRecord{}).Where("id = ?", id).Update("is_favorite", isFavorite).Error
}

func (r *aiGenerationRecordRepo) UpdateRating(id uint, rating int) error {
	return r.db.Model(&model.AIGenerationRecord{}).Where("id = ?", id).Update("rating", rating).Error
}

func (r *aiGenerationRecordRepo) Delete(id uint) error {
	return r.db.Delete(&model.AIGenerationRecord{}, id).Error
}

func (r *aiGenerationRecordRepo) CountByMerchantAndUser(userID uint) (int64, error) {
	var count int64
	err := r.db.Model(&model.AIGenerationRecord{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}

// PromptTemplateRepository 提示词模板仓库接口
type PromptTemplateRepository interface {
	Create(template *model.PromptTemplate) error
	GetByID(id uint) (*model.PromptTemplate, error)
	ListByType(templateType string) ([]*model.PromptTemplate, error)
	GetByTypeAndName(templateType model.AIGenerationType, name string) (*model.PromptTemplate, error)
	Update(template *model.PromptTemplate) error
	Delete(id uint) error
	IncrementUseCount(id uint) error
}

type promptTemplateRepo struct {
	db *gorm.DB
}

// NewPromptTemplateRepository 创建提示词模板仓库实例
func NewPromptTemplateRepository(db *gorm.DB) PromptTemplateRepository {
	return &promptTemplateRepo{db: db}
}

func (r *promptTemplateRepo) Create(template *model.PromptTemplate) error {
	return r.db.Create(template).Error
}

func (r *promptTemplateRepo) GetByID(id uint) (*model.PromptTemplate, error) {
	var template model.PromptTemplate
	err := r.db.First(&template, id).Error
	return &template, err
}

func (r *promptTemplateRepo) ListByType(templateType string) ([]*model.PromptTemplate, error) {
	var templates []*model.PromptTemplate

	query := r.db.Model(&model.PromptTemplate{}).Where("status = ?", 1)

	// 独立部署：直接返回所有模板（系统模板 + 用户自有模板）
	if templateType != "" {
		query = query.Where("type = ?", templateType)
	}

	err := query.Order("is_system DESC, created_at ASC").Find(&templates).Error
	return templates, err
}

func (r *promptTemplateRepo) GetByTypeAndName(templateType model.AIGenerationType, name string) (*model.PromptTemplate, error) {
	var template model.PromptTemplate
	err := r.db.Where("type = ? AND name = ?", templateType, name).First(&template).Error
	if err != nil {
		return nil, err
	}
	return &template, nil
}

func (r *promptTemplateRepo) Update(template *model.PromptTemplate) error {
	return r.db.Save(template).Error
}

func (r *promptTemplateRepo) Delete(id uint) error {
	return r.db.Delete(&model.PromptTemplate{}, id).Error
}

func (r *promptTemplateRepo) IncrementUseCount(id uint) error {
	return r.db.Model(&model.PromptTemplate{}).Where("id = ?", id).
		UpdateColumn("use_count", gorm.Expr("use_count + ?", 1)).Error
}
