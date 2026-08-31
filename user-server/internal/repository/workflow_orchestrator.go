package repository

import (
	"context"
	"errors"
	"time"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// WorkflowVersionRepository 工作流版本仓储
type WorkflowVersionRepository struct {
	db *gorm.DB
}

// NewWorkflowVersionRepository 创建工作流版本仓储
func NewWorkflowVersionRepository(db *gorm.DB) *WorkflowVersionRepository {
	return &WorkflowVersionRepository{db: db}
}

// Save 保存工作流版本（存在则更新，不存在则创建）
func (r *WorkflowVersionRepository) Save(ctx context.Context, w *model.WorkflowVersion) error {
	if r == nil || r.db == nil {
		return errors.New("workflow version repository not initialized")
	}
	return r.db.WithContext(ctx).Save(w).Error
}

// GetByID 按 ID 获取工作流版本
func (r *WorkflowVersionRepository) GetByID(ctx context.Context, id uint) (*model.WorkflowVersion, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("workflow version repository not initialized")
	}
	var w model.WorkflowVersion
	if err := r.db.WithContext(ctx).First(&w, id).Error; err != nil {
		return nil, err
	}
	return &w, nil
}

// GetLatestPublished 按 workflow_id 获取最新的已发布版本
func (r *WorkflowVersionRepository) GetLatestPublished(ctx context.Context, workflowID string) (*model.WorkflowVersion, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("workflow version repository not initialized")
	}
	var w model.WorkflowVersion
	if err := r.db.WithContext(ctx).
		Where("workflow_id = ? AND status = ?", workflowID, model.WorkflowStatusPublished).
		Order("version DESC").
		First(&w).Error; err != nil {
		return nil, err
	}
	return &w, nil
}

// GetByWorkflowIDAndVersion 按 workflow_id + version 精确获取版本（dispatcher 加载定义用）
func (r *WorkflowVersionRepository) GetByWorkflowIDAndVersion(ctx context.Context, workflowID string, version int) (*model.WorkflowVersion, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("workflow version repository not initialized")
	}
	var w model.WorkflowVersion
	if err := r.db.WithContext(ctx).
		Where("workflow_id = ? AND version = ?", workflowID, version).
		First(&w).Error; err != nil {
		return nil, err
	}
	return &w, nil
}

// ListVersions 按 workflow_id 列出所有版本（按 version DESC 排序）
func (r *WorkflowVersionRepository) ListVersions(ctx context.Context, workflowID string) ([]model.WorkflowVersion, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("workflow version repository not initialized")
	}
	var list []model.WorkflowVersion
	if err := r.db.WithContext(ctx).
		Where("workflow_id = ?", workflowID).
		Order("version DESC").
		Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ListAll 列出所有工作流版本，支持按 workflow_id 和 status 过滤，带分页
func (r *WorkflowVersionRepository) ListAll(ctx context.Context, workflowID, status string, page, pageSize int) ([]model.WorkflowVersion, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, errors.New("workflow version repository not initialized")
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	q := r.db.WithContext(ctx).Model(&model.WorkflowVersion{})
	if workflowID != "" {
		q = q.Where("workflow_id = ?", workflowID)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.WorkflowVersion
	err := q.Order("updated_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&list).Error
	return list, total, err
}

// DeleteByID 按 ID 删除工作流版本
func (r *WorkflowVersionRepository) DeleteByID(ctx context.Context, id uint) error {
	if r == nil || r.db == nil {
		return errors.New("workflow version repository not initialized")
	}
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.WorkflowVersion{}).Error
}

// UpdateStatus 更新版本状态
func (r *WorkflowVersionRepository) UpdateStatus(ctx context.Context, id uint, status string) error {
	if r == nil || r.db == nil {
		return errors.New("workflow version repository not initialized")
	}
	return r.db.WithContext(ctx).Model(&model.WorkflowVersion{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// CountByWorkflowID 统计指定 workflow_id 的版本数（用于自增 version）
func (r *WorkflowVersionRepository) CountByWorkflowID(ctx context.Context, workflowID string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("workflow version repository not initialized")
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.WorkflowVersion{}).
		Where("workflow_id = ?", workflowID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// WorkflowExecutionRepository 工作流执行实例仓储
type WorkflowExecutionRepository struct {
	db *gorm.DB
}

// NewWorkflowExecutionRepository 创建工作流执行实例仓储
func NewWorkflowExecutionRepository(db *gorm.DB) *WorkflowExecutionRepository {
	return &WorkflowExecutionRepository{db: db}
}

// Create 创建执行实例
func (r *WorkflowExecutionRepository) Create(ctx context.Context, e *model.WorkflowExecution) error {
	if r == nil || r.db == nil {
		return errors.New("workflow execution repository not initialized")
	}
	return r.db.WithContext(ctx).Create(e).Error
}

// GetByID 按 ID 获取执行实例
func (r *WorkflowExecutionRepository) GetByID(ctx context.Context, id uint) (*model.WorkflowExecution, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("workflow execution repository not initialized")
	}
	var e model.WorkflowExecution
	if err := r.db.WithContext(ctx).First(&e, id).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

// Update 更新执行实例
func (r *WorkflowExecutionRepository) Update(ctx context.Context, e *model.WorkflowExecution) error {
	if r == nil || r.db == nil {
		return errors.New("workflow execution repository not initialized")
	}
	return r.db.WithContext(ctx).Save(e).Error
}

// UpdateFields 按字段更新
func (r *WorkflowExecutionRepository) UpdateFields(ctx context.Context, id uint, fields map[string]any) error {
	if r == nil || r.db == nil {
		return errors.New("workflow execution repository not initialized")
	}
	return r.db.WithContext(ctx).Model(&model.WorkflowExecution{}).
		Where("id = ?", id).
		Updates(fields).Error
}

// List 列出执行实例
func (r *WorkflowExecutionRepository) List(ctx context.Context, workflowID, status string, page, pageSize int) ([]model.WorkflowExecution, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, errors.New("workflow execution repository not initialized")
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	q := r.db.WithContext(ctx).Model(&model.WorkflowExecution{})
	if workflowID != "" {
		q = q.Where("workflow_id = ?", workflowID)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.WorkflowExecution
	err := q.Order("started_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&list).Error
	return list, total, err
}

// ListByWorkflowID 按 workflow_id 列出执行实例（不分页，用于详情）
func (r *WorkflowExecutionRepository) ListByWorkflowID(ctx context.Context, workflowID string, limit int) ([]model.WorkflowExecution, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("workflow execution repository not initialized")
	}
	if limit <= 0 {
		limit = 50
	}
	var list []model.WorkflowExecution
	if err := r.db.WithContext(ctx).
		Where("workflow_id = ?", workflowID).
		Order("started_at DESC").
		Limit(limit).
		Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// FindStuck 查询卡死的执行实例
func (r *WorkflowExecutionRepository) FindStuck(ctx context.Context, threshold time.Time, limit int) ([]model.WorkflowExecution, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("workflow execution repository not initialized")
	}
	if limit <= 0 {
		limit = 50
	}
	var list []model.WorkflowExecution
	err := r.db.WithContext(ctx).
		Where("status = ? AND started_at < ?", model.WorkflowExecRunning, threshold).
		Order("started_at ASC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

// WorkflowNodeExecutionRepository 节点执行明细仓储
type WorkflowNodeExecutionRepository struct {
	db *gorm.DB
}

// NewWorkflowNodeExecutionRepository 创建节点执行明细仓储
func NewWorkflowNodeExecutionRepository(db *gorm.DB) *WorkflowNodeExecutionRepository {
	return &WorkflowNodeExecutionRepository{db: db}
}

// Create 创建节点执行记录
func (r *WorkflowNodeExecutionRepository) Create(ctx context.Context, n *model.WorkflowNodeExecution) error {
	if r == nil || r.db == nil {
		return errors.New("workflow node execution repository not initialized")
	}
	return r.db.WithContext(ctx).Create(n).Error
}

// Update 更新节点执行记录
func (r *WorkflowNodeExecutionRepository) Update(ctx context.Context, n *model.WorkflowNodeExecution) error {
	if r == nil || r.db == nil {
		return errors.New("workflow node execution repository not initialized")
	}
	return r.db.WithContext(ctx).Save(n).Error
}

// ListByExecutionID 按 execution_id 列出节点执行明细（按 started_at ASC）
func (r *WorkflowNodeExecutionRepository) ListByExecutionID(ctx context.Context, executionID uint) ([]model.WorkflowNodeExecution, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("workflow node execution repository not initialized")
	}
	var list []model.WorkflowNodeExecution
	if err := r.db.WithContext(ctx).
		Where("execution_id = ?", executionID).
		Order("started_at ASC").
		Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// CountByExecutionIDAndStatus 统计节点执行状态
func (r *WorkflowNodeExecutionRepository) CountByExecutionIDAndStatus(ctx context.Context, executionID uint, status string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("workflow node execution repository not initialized")
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.WorkflowNodeExecution{}).
		Where("execution_id = ? AND status = ?", executionID, status).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
