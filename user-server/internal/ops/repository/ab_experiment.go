package repository

import (
	"hivemtk-user/internal/ops/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// ABExperimentRepository A/B 实验仓库
type ABExperimentRepository struct {
	db *gorm.DB
}

// NewABExperimentRepository 创建 A/B 实验仓库实例
func NewABExperimentRepository() *ABExperimentRepository {
	return &ABExperimentRepository{
		db: _db.GetDB(),
	}
}

// Create 创建实验
func (r *ABExperimentRepository) Create(experiment *model.ABExperiment) error {
	return r.db.Create(experiment).Error
}

// GetByID 根据 ID 获取实验
func (r *ABExperimentRepository) GetByID(id uint) (*model.ABExperiment, error) {
	var experiment model.ABExperiment
	err := r.db.First(&experiment, id).Error
	return &experiment, err
}

// GetAll 获取所有实验列表
func (r *ABExperimentRepository) GetAll(page, pageSize int) ([]*model.ABExperiment, int64, error) {
	var experiments []*model.ABExperiment
	var total int64

	r.db.Model(&model.ABExperiment{}).Count(&total)
	err := r.db.
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&experiments).Error

	return experiments, total, err
}

// Update 更新实验
func (r *ABExperimentRepository) Update(experiment *model.ABExperiment) error {
	return r.db.Save(experiment).Error
}

// Delete 删除实验
func (r *ABExperimentRepository) Delete(id uint) error {
	return r.db.Delete(&model.ABExperiment{}, id).Error
}

// UpdateStatus 更新实验状态
func (r *ABExperimentRepository) UpdateStatus(id uint, status string) error {
	return r.db.Model(&model.ABExperiment{}).Where("id = ?", id).Update("status", status).Error
}

// GetRunningBySourceID 根据 sourceID 获取运行中的实验
func (r *ABExperimentRepository) GetRunningBySourceID(sourceID string) (*model.ABExperiment, error) {
	var experiment model.ABExperiment
	err := r.db.Where("source_id = ? AND status = ?", sourceID, "running").First(&experiment).Error
	return &experiment, err
}

// GetLatestBySourceID 根据 sourceID 获取最新实验（不限状态，按 updated_at DESC）
func (r *ABExperimentRepository) GetLatestBySourceID(sourceID string) (*model.ABExperiment, error) {
	var experiment model.ABExperiment
	err := r.db.Where("source_id = ?", sourceID).
		Order("updated_at DESC").
		First(&experiment).Error
	return &experiment, err
}

// ABVariantRepository A/B 变体仓库
type ABVariantRepository struct {
	db *gorm.DB
}

// NewABVariantRepository 创建 A/B 变体仓库实例
func NewABVariantRepository() *ABVariantRepository {
	return &ABVariantRepository{
		db: _db.GetDB(),
	}
}

// Create 创建变体
func (r *ABVariantRepository) Create(variant *model.ABVariant) error {
	return r.db.Create(variant).Error
}

// GetByID 根据 ID 获取变体
func (r *ABVariantRepository) GetByID(id uint) (*model.ABVariant, error) {
	var variant model.ABVariant
	err := r.db.First(&variant, id).Error
	return &variant, err
}

// GetByExperiment 获取实验下的变体列表
// v3 审计 P2-8：必须按 id 稳定排序——流量桶按切片顺序累积划分，
// 无序返回会导致桶边界随数据库返回顺序漂移，同一用户被分到不同变体。
func (r *ABVariantRepository) GetByExperiment(experimentID uint) ([]*model.ABVariant, error) {
	var variants []*model.ABVariant
	err := r.db.Where("experiment_id = ?", experimentID).
		Order("id ASC").Find(&variants).Error
	return variants, err
}

// Update 更新变体
func (r *ABVariantRepository) Update(variant *model.ABVariant) error {
	return r.db.Save(variant).Error
}

// Delete 删除变体
func (r *ABVariantRepository) Delete(id uint) error {
	return r.db.Delete(&model.ABVariant{}, id).Error
}

// DeleteByExperiment 删除实验下所有变体
func (r *ABVariantRepository) DeleteByExperiment(experimentID uint) error {
	return r.db.Where("experiment_id = ?", experimentID).Delete(&model.ABVariant{}).Error
}

// IncrementTraffic 增加访问计数
func (r *ABVariantRepository) IncrementTraffic(id uint) error {
	return r.db.Model(&model.ABVariant{}).Where("id = ?", id).
		UpdateColumn("traffic_count", gorm.Expr("traffic_count + 1")).Error
}

// IncrementConversion 增加转化计数
func (r *ABVariantRepository) IncrementConversion(id uint) error {
	return r.db.Model(&model.ABVariant{}).Where("id = ?", id).
		UpdateColumn("conversion_count", gorm.Expr("conversion_count + 1")).Error
}

// ABConversionEventRepository 转化事件仓库
type ABConversionEventRepository struct {
	db *gorm.DB
}

// NewABConversionEventRepository 创建转化事件仓库实例
func NewABConversionEventRepository() *ABConversionEventRepository {
	return &ABConversionEventRepository{
		db: _db.GetDB(),
	}
}

// Create 创建转化事件
func (r *ABConversionEventRepository) Create(event *model.ABConversionEvent) error {
	return r.db.Create(event).Error
}

// GetByExperiment 获取实验下的转化事件列表
func (r *ABConversionEventRepository) GetByExperiment(experimentID uint, page, pageSize int) ([]*model.ABConversionEvent, int64, error) {
	var events []*model.ABConversionEvent
	var total int64

	r.db.Model(&model.ABConversionEvent{}).Where("experiment_id = ?", experimentID).Count(&total)
	err := r.db.Where("experiment_id = ?", experimentID).
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&events).Error

	return events, total, err
}

// GetByVariant 获取变体下的转化事件
func (r *ABConversionEventRepository) GetByVariant(variantID uint) ([]*model.ABConversionEvent, error) {
	var events []*model.ABConversionEvent
	err := r.db.Where("variant_id = ?", variantID).Find(&events).Error
	return events, err
}

// ABExperimentResultRepository 实验结果仓库
type ABExperimentResultRepository struct {
	db *gorm.DB
}

// NewABExperimentResultRepository 创建实验结果仓库实例
func NewABExperimentResultRepository() *ABExperimentResultRepository {
	return &ABExperimentResultRepository{
		db: _db.GetDB(),
	}
}

// Upsert 创建或更新结果
func (r *ABExperimentResultRepository) Upsert(result *model.ABExperimentResult) error {
	var existing model.ABExperimentResult
	err := r.db.Where("experiment_id = ? AND variant_id = ?", result.ExperimentID, result.VariantID).First(&existing).Error
	if err == nil {
		result.ID = existing.ID
		return r.db.Save(result).Error
	}
	return r.db.Create(result).Error
}

// GetByExperiment 获取实验结果
func (r *ABExperimentResultRepository) GetByExperiment(experimentID uint) ([]*model.ABExperimentResult, error) {
	var results []*model.ABExperimentResult
	err := r.db.Where("experiment_id = ?", experimentID).Find(&results).Error
	return results, err
}

// UpdateWinner 更新获胜者
func (r *ABExperimentResultRepository) UpdateWinner(experimentID uint, winnerVariantID uint) error {
	r.db.Model(&model.ABExperimentResult{}).Where("experiment_id = ?", experimentID).
		Update("is_winner", false)

	return r.db.Model(&model.ABExperimentResult{}).Where("experiment_id = ? AND variant_id = ?", experimentID, winnerVariantID).
		Update("is_winner", true).Error
}

