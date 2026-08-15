package repository

import (
	"hivemtk-user/internal/ops/model"
	_db "hivemtk-user/internal/pkg/db"
	"time"

	"gorm.io/gorm"
)

// ChurnPredictionRepository 流失预测仓库
type ChurnPredictionRepository struct {
	db *gorm.DB
}

// NewChurnPredictionRepository 创建流失预测仓库实例
func NewChurnPredictionRepository() *ChurnPredictionRepository {
	return &ChurnPredictionRepository{
		db: _db.GetDB(),
	}
}

// Upsert 创建或更新流失预测
func (r *ChurnPredictionRepository) Upsert(prediction *model.ChurnPrediction) error {
	var existing model.ChurnPrediction
	err := r.db.Where("user_id = ?", prediction.UserID).First(&existing).Error
	if err == nil {
		prediction.ID = existing.ID
		return r.db.Save(prediction).Error
	}
	return r.db.Create(prediction).Error
}

// GetByID 根据 ID 获取流失预测
func (r *ChurnPredictionRepository) GetByID(id uint) (*model.ChurnPrediction, error) {
	var prediction model.ChurnPrediction
	err := r.db.First(&prediction, id).Error
	return &prediction, err
}

// GetAll 获取所有流失预测列表
func (r *ChurnPredictionRepository) GetAll(page, pageSize int) ([]*model.ChurnPrediction, int64, error) {
	var predictions []*model.ChurnPrediction
	var total int64

	r.db.Model(&model.ChurnPrediction{}).Count(&total)
	err := r.db.
		Order("churn_score DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&predictions).Error

	return predictions, total, err
}

// GetByRisk 根据风险等级获取流失预测
func (r *ChurnPredictionRepository) GetByRisk(riskLevel string, page, pageSize int) ([]*model.ChurnPrediction, int64, error) {
	var predictions []*model.ChurnPrediction
	var total int64

	r.db.Model(&model.ChurnPrediction{}).Where("churn_risk = ?", riskLevel).Count(&total)
	err := r.db.Where("churn_risk = ?", riskLevel).
		Order("churn_score DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&predictions).Error

	return predictions, total, err
}

// GetByUserID 根据用户 ID 获取流失预测
func (r *ChurnPredictionRepository) GetByUserID(userID string) (*model.ChurnPrediction, error) {
	var prediction model.ChurnPrediction
	err := r.db.Where("user_id = ?", userID).First(&prediction).Error
	return &prediction, err
}

// GetHighRiskUsers 获取高风险用户列表
func (r *ChurnPredictionRepository) GetHighRiskUsers(limit int) ([]*model.ChurnPrediction, error) {
	var predictions []*model.ChurnPrediction
	err := r.db.Where("churn_risk IN ?", []string{"high", "critical"}).
		Order("churn_score DESC").
		Limit(limit).
		Find(&predictions).Error
	return predictions, err
}

// ChurnWarningRepository 流失预警仓库
type ChurnWarningRepository struct {
	db *gorm.DB
}

// NewChurnWarningRepository 创建流失预警仓库实例
func NewChurnWarningRepository() *ChurnWarningRepository {
	return &ChurnWarningRepository{
		db: _db.GetDB(),
	}
}

// Create 创建流失预警
func (r *ChurnWarningRepository) Create(warning *model.ChurnWarning) error {
	return r.db.Create(warning).Error
}

// GetByID 根据 ID 获取流失预警
func (r *ChurnWarningRepository) GetByID(id uint) (*model.ChurnWarning, error) {
	var warning model.ChurnWarning
	err := r.db.First(&warning, id).Error
	return &warning, err
}

// GetAll 获取所有流失预警列表
func (r *ChurnWarningRepository) GetAll(page, pageSize int) ([]*model.ChurnWarning, int64, error) {
	var warnings []*model.ChurnWarning
	var total int64

	r.db.Model(&model.ChurnWarning{}).Count(&total)
	err := r.db.
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&warnings).Error

	return warnings, total, err
}

// GetUnhandled 获取未处理的流失预警
func (r *ChurnWarningRepository) GetUnhandled(page, pageSize int) ([]*model.ChurnWarning, int64, error) {
	var warnings []*model.ChurnWarning
	var total int64

	r.db.Model(&model.ChurnWarning{}).Where("is_handled = ?", false).Count(&total)
	err := r.db.Where("is_handled = ?", false).
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&warnings).Error

	return warnings, total, err
}

// Update 更新流失预警
func (r *ChurnWarningRepository) Update(warning *model.ChurnWarning) error {
	return r.db.Save(warning).Error
}

// MarkHandled 标记为已处理
func (r *ChurnWarningRepository) MarkHandled(id uint, handledBy uint, note string) error {
	now := time.Now()
	return r.db.Model(&model.ChurnWarning{}).Where("id = ?", id).Updates(map[string]any{
		"is_handled":   true,
		"handled_at":   now,
		"handled_by":   handledBy,
		"handled_note": note,
		"updated_at":   now,
	}).Error
}

// ChurnModelConfigRepository 流失模型配置仓库
type ChurnModelConfigRepository struct {
	db *gorm.DB
}

// NewChurnModelConfigRepository 创建流失模型配置仓库实例
func NewChurnModelConfigRepository() *ChurnModelConfigRepository {
	return &ChurnModelConfigRepository{
		db: _db.GetDB(),
	}
}

// GetCurrent 获取当前模型配置
func (r *ChurnModelConfigRepository) GetCurrent() (*model.ChurnModelConfig, error) {
	var config model.ChurnModelConfig
	err := r.db.First(&config).Error
	return &config, err
}

// Upsert 创建或更新模型配置
func (r *ChurnModelConfigRepository) Upsert(config *model.ChurnModelConfig) error {
	var existing model.ChurnModelConfig
	err := r.db.Where("id > ?", 0).First(&existing).Error
	if err == nil {
		config.ID = existing.ID
		return r.db.Save(config).Error
	}
	return r.db.Create(config).Error
}

// UpdateCalcTime 更新最后计算时间
func (r *ChurnModelConfigRepository) UpdateCalcTime() error {
	return r.db.Model(&model.ChurnModelConfig{}).
		Where("id > ?", 0).
		Update("last_calculated_at", time.Now()).Error
}

// ChurnStatisticsRepository 流失统计仓库
type ChurnStatisticsRepository struct {
	db *gorm.DB
}

// NewChurnStatisticsRepository 创建流失统计仓库实例
func NewChurnStatisticsRepository() *ChurnStatisticsRepository {
	return &ChurnStatisticsRepository{
		db: _db.GetDB(),
	}
}

// Create 创建流失统计
func (r *ChurnStatisticsRepository) Create(stats *model.ChurnStatistics) error {
	return r.db.Create(stats).Error
}

// GetAll 获取所有流失统计
func (r *ChurnStatisticsRepository) GetAll(startDate, endDate string) ([]*model.ChurnStatistics, error) {
	var stats []*model.ChurnStatistics
	err := r.db.Where("stat_date BETWEEN ? AND ?", startDate, endDate).
		Order("stat_date DESC").
		Find(&stats).Error
	return stats, err
}

// GetLatest 获取最新的流失统计
func (r *ChurnStatisticsRepository) GetLatest() (*model.ChurnStatistics, error) {
	var stats model.ChurnStatistics
	err := r.db.
		Order("stat_date DESC").
		First(&stats).Error
	return &stats, err
}

// NewChurnPredictionRepositoryWithDB 创建指定数据库连接的流失预测仓库实例（用于测试）
func NewChurnPredictionRepositoryWithDB(db *gorm.DB) *ChurnPredictionRepository {
	return &ChurnPredictionRepository{db: db}
}

// NewChurnWarningRepositoryWithDB 创建指定数据库连接的流失预警仓库实例（用于测试）
func NewChurnWarningRepositoryWithDB(db *gorm.DB) *ChurnWarningRepository {
	return &ChurnWarningRepository{db: db}
}

// NewChurnModelConfigRepositoryWithDB 创建指定数据库连接的流失模型配置仓库实例（用于测试）
func NewChurnModelConfigRepositoryWithDB(db *gorm.DB) *ChurnModelConfigRepository {
	return &ChurnModelConfigRepository{db: db}
}

// NewChurnStatisticsRepositoryWithDB 创建指定数据库连接的流失统计仓库实例（用于测试）
func NewChurnStatisticsRepositoryWithDB(db *gorm.DB) *ChurnStatisticsRepository {
	return &ChurnStatisticsRepository{db: db}
}

