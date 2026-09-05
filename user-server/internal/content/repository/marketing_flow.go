package repository

import (
	"hivemtk-user/internal/content/model"
	_db "hivemtk-user/internal/pkg/db"
	"time"

	"gorm.io/gorm"
)

// MarketingFlowRepository 营销流程仓库
type MarketingFlowRepository struct {
	db *gorm.DB
}

// NewMarketingFlowRepository 创建营销流程仓库实例
func NewMarketingFlowRepository() *MarketingFlowRepository {
	return &MarketingFlowRepository{
		db: _db.GetDB(),
	}
}

// NewMarketingFlowRepositoryWithDB 创建营销流程仓库实例（带数据库连接，用于测试）
func NewMarketingFlowRepositoryWithDB(db *gorm.DB) *MarketingFlowRepository {
	return &MarketingFlowRepository{
		db: db,
	}
}

// Create 创建流程
func (r *MarketingFlowRepository) Create(flow *model.MarketingFlow) error {
	return r.db.Create(flow).Error
}

// GetByID 根据 ID 获取流程
func (r *MarketingFlowRepository) GetByID(id uint) (*model.MarketingFlow, error) {
	var flow model.MarketingFlow
	err := r.db.First(&flow, id).Error
	return &flow, err
}

// GetAll 获取所有流程列表
func (r *MarketingFlowRepository) GetAll(page, pageSize int) ([]*model.MarketingFlow, int64, error) {
	var flows []*model.MarketingFlow
	var total int64

	r.db.Model(&model.MarketingFlow{}).Count(&total)
	err := r.db.
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&flows).Error

	return flows, total, err
}

// Update 更新流程
func (r *MarketingFlowRepository) Update(flow *model.MarketingFlow) error {
	return r.db.Save(flow).Error
}

// Delete 删除流程
func (r *MarketingFlowRepository) Delete(id uint) error {
	return r.db.Delete(&model.MarketingFlow{}, id).Error
}

// GetByStatus 根据状态获取流程
func (r *MarketingFlowRepository) GetByStatus(status model.FlowStatus) ([]*model.MarketingFlow, error) {
	var flows []*model.MarketingFlow
	err := r.db.Where("status = ?", status).Find(&flows).Error
	return flows, err
}

// UpdateStatus 更新流程状态
func (r *MarketingFlowRepository) UpdateStatus(id uint, status model.FlowStatus) error {
	return r.db.Model(&model.MarketingFlow{}).Where("id = ?", id).Update("status", status).Error
}

// FlowExecutionRepository 流程执行仓库
type FlowExecutionRepository struct {
	db *gorm.DB
}

// NewFlowExecutionRepository 创建流程执行仓库实例
func NewFlowExecutionRepository() *FlowExecutionRepository {
	return &FlowExecutionRepository{
		db: _db.GetDB(),
	}
}

// NewFlowExecutionRepositoryWithDB 创建流程执行仓库实例（带数据库连接，用于测试）
func NewFlowExecutionRepositoryWithDB(db *gorm.DB) *FlowExecutionRepository {
	return &FlowExecutionRepository{
		db: db,
	}
}

// Create 创建执行记录
func (r *FlowExecutionRepository) Create(execution *model.FlowExecution) error {
	return r.db.Create(execution).Error
}

// GetByID 根据 ID 获取执行记录
func (r *FlowExecutionRepository) GetByID(id uint) (*model.FlowExecution, error) {
	var execution model.FlowExecution
	err := r.db.First(&execution, id).Error
	return &execution, err
}

// GetByFlowID 根据流程 ID 获取执行记录
func (r *FlowExecutionRepository) GetByFlowID(flowID uint, page, pageSize int) ([]*model.FlowExecution, int64, error) {
	var executions []*model.FlowExecution
	var total int64

	r.db.Model(&model.FlowExecution{}).Where("flow_id = ?", flowID).Count(&total)
	err := r.db.Where("flow_id = ?", flowID).
		Order("started_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&executions).Error

	return executions, total, err
}

// Update 更新执行记录
func (r *FlowExecutionRepository) Update(execution *model.FlowExecution) error {
	return r.db.Save(execution).Error
}

// GetRunningExecutions 获取运行中的执行记录
func (r *FlowExecutionRepository) GetRunningExecutions(flowID uint) ([]*model.FlowExecution, error) {
	var executions []*model.FlowExecution
	err := r.db.Where("flow_id = ? AND status = ?", flowID, "running").Find(&executions).Error
	return executions, err
}

// CreateWithTx 在事务中创建执行记录
func (r *FlowExecutionRepository) CreateWithTx(tx *gorm.DB, execution *model.FlowExecution) error {
	return tx.Create(execution).Error
}

// GetByUser 根据用户 ID 获取执行记录
func (r *FlowExecutionRepository) GetByUser(userID string, page, pageSize int) ([]*model.FlowExecution, int64, error) {
	var executions []*model.FlowExecution
	var total int64

	r.db.Model(&model.FlowExecution{}).Where("user_id = ?", userID).Count(&total)
	err := r.db.Where("user_id = ?", userID).
		Order("started_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&executions).Error

	return executions, total, err
}

// GetStats 获取执行统计
func (r *FlowExecutionRepository) GetStats(flowID uint) (map[string]int64, error) {
	var stats []struct {
		Status string `gorm:"column:status"`
		Count  int64  `gorm:"column:count"`
	}

	err := r.db.Model(&model.FlowExecution{}).
		Select("status, COUNT(*) as count").
		Where("flow_id = ?", flowID).
		Group("status").
		Scan(&stats).Error

	result := make(map[string]int64)
	for _, s := range stats {
		result[s.Status] = s.Count
	}
	return result, err
}

// CleanupOldExecutions 清理旧的执行记录（保留最近 30 天）
func (r *FlowExecutionRepository) CleanupOldExecutions() error {
	threshold := time.Now().AddDate(0, 0, -30)
	return r.db.Where("completed_at < ? AND status IN ?", threshold, []string{"completed", "failed", "cancelled"}).
		Delete(&model.FlowExecution{}).Error
}
