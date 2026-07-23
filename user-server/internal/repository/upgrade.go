package repository

import (
	"context"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
	"time"

	"gorm.io/gorm"
)

// UpgradeTaskRepository 升级任务仓库
type UpgradeTaskRepository struct {
	db *gorm.DB
}

// NewUpgradeTaskRepository 创建升级任务仓库实例
func NewUpgradeTaskRepository() *UpgradeTaskRepository {
	return &UpgradeTaskRepository{
		db: _db.GetDB(),
	}
}

// Create 创建升级任务
func (r *UpgradeTaskRepository) Create(ctx context.Context, task *model.UpgradeTask) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// GetByID 根据 ID 获取升级任务
func (r *UpgradeTaskRepository) GetByID(ctx context.Context, id uint) (*model.UpgradeTask, error) {
	var task model.UpgradeTask
	err := r.db.WithContext(ctx).First(&task, id).Error
	return &task, err
}

func (r *UpgradeTaskRepository) GetAll(ctx context.Context, page, pageSize int) ([]*model.UpgradeTask, int64, error) {
	var tasks []*model.UpgradeTask
	var total int64

	db := r.db.WithContext(ctx)
	db.Model(&model.UpgradeTask{}).Count(&total)
	err := db.Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&tasks).Error

	return tasks, total, err
}

// GetLatestTask 获取最新的升级任务（独立部署版本）
func (r *UpgradeTaskRepository) GetLatestTask(ctx context.Context) (*model.UpgradeTask, error) {
	var task model.UpgradeTask
	err := r.db.WithContext(ctx).Order("created_at DESC").First(&task).Error
	return &task, err
}

// Update 更新升级任务
func (r *UpgradeTaskRepository) Update(ctx context.Context, task *model.UpgradeTask) error {
	return r.db.WithContext(ctx).Save(task).Error
}

// UpdateStatus 更新升级任务状态
func (r *UpgradeTaskRepository) UpdateStatus(ctx context.Context, id uint, status string, progress int, currentStep int, stepDesc string, errorMessage string) error {
	updates := map[string]any{
		"status":            status,
		"progress":          progress,
		"current_step":      currentStep,
		"current_step_desc": stepDesc,
		"error_message":     errorMessage,
	}
	if status == "completed" {
		now := time.Now()
		updates["completed_at"] = now
	}
	return r.db.WithContext(ctx).Model(&model.UpgradeTask{}).Where("id = ?", id).Updates(updates).Error
}

// MigrationRecordRepository 迁移记录仓库
type MigrationRecordRepository struct {
	db *gorm.DB
}

// NewMigrationRecordRepository 创建迁移记录仓库实例
func NewMigrationRecordRepository() *MigrationRecordRepository {
	return &MigrationRecordRepository{
		db: _db.GetDB(),
	}
}

// Create 创建迁移记录
func (r *MigrationRecordRepository) Create(ctx context.Context, record *model.MigrationRecord) error {
	return r.db.WithContext(ctx).Create(record).Error
}

func (r *MigrationRecordRepository) GetAll(ctx context.Context) ([]*model.MigrationRecord, error) {
	var records []*model.MigrationRecord
	err := r.db.WithContext(ctx).Order("executed_at DESC").Find(&records).Error
	return records, err
}

// GetByVersion 根据版本获取迁移记录
func (r *MigrationRecordRepository) GetByVersion(ctx context.Context, version string) (*model.MigrationRecord, error) {
	var record model.MigrationRecord
	err := r.db.WithContext(ctx).Where("version = ?", version).First(&record).Error
	return &record, err
}

func (r *MigrationRecordRepository) GetExecutedVersions(ctx context.Context) ([]string, error) {
	var versions []string
	err := r.db.WithContext(ctx).Model(&model.MigrationRecord{}).
		Where("status = ?", "completed").
		Pluck("version", &versions).Error
	return versions, err
}

// Update 更新迁移记录
func (r *MigrationRecordRepository) Update(ctx context.Context, record *model.MigrationRecord) error {
	return r.db.WithContext(ctx).Save(record).Error
}

// MigrationCheckpointRepository 迁移检查点仓库
type MigrationCheckpointRepository struct {
	db *gorm.DB
}

// NewMigrationCheckpointRepository 创建迁移检查点仓库实例
func NewMigrationCheckpointRepository() *MigrationCheckpointRepository {
	return &MigrationCheckpointRepository{
		db: _db.GetDB(),
	}
}

// GetByCheckpoint 根据检查点名称获取
func (r *MigrationCheckpointRepository) GetByCheckpoint(ctx context.Context, checkpoint string) (*model.MigrationCheckpoint, error) {
	var cp model.MigrationCheckpoint
	err := r.db.WithContext(ctx).Where("checkpoint = ?", checkpoint).First(&cp).Error
	return &cp, err
}

func (r *MigrationCheckpointRepository) GetAll(ctx context.Context) ([]*model.MigrationCheckpoint, error) {
	var checkpoints []*model.MigrationCheckpoint
	err := r.db.WithContext(ctx).Find(&checkpoints).Error
	return checkpoints, err
}

// Upsert 创建或更新检查点
func (r *MigrationCheckpointRepository) Upsert(ctx context.Context, checkpoint *model.MigrationCheckpoint) error {
	existing, err := r.GetByCheckpoint(ctx, checkpoint.Checkpoint)
	if err == nil && existing != nil {
		checkpoint.ID = existing.ID
		return r.db.WithContext(ctx).Save(checkpoint).Error
	}
	return r.db.WithContext(ctx).Create(checkpoint).Error
}
