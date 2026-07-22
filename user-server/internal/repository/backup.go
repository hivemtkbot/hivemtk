package repository

import (
	"context"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
	"time"

	"gorm.io/gorm"
)

// BackupRepository 备份仓库
//
// 2026-07-22 方向E：所有方法第一参数改为 ctx context.Context，透传至 r.db.WithContext(ctx)，
// 保证 trace / timeout / cancel 在整条调用链上贯通。
type BackupRepository struct {
	db *gorm.DB
}

// NewBackupRepository 创建备份仓库实例
func NewBackupRepository() *BackupRepository {
	return &BackupRepository{
		db: _db.GetDB(),
	}
}

// NewBackupRepositoryWithDB 使用指定数据库创建备份仓库实例（用于测试）
func NewBackupRepositoryWithDB(database *gorm.DB) *BackupRepository {
	return &BackupRepository{
		db: database,
	}
}

// Create 创建备份记录
func (r *BackupRepository) Create(ctx context.Context, backup *model.Backup) error {
	return r.db.WithContext(ctx).Create(backup).Error
}

// GetByID 根据 ID 获取备份记录
func (r *BackupRepository) GetByID(ctx context.Context, id uint) (*model.Backup, error) {
	var backup model.Backup
	err := r.db.WithContext(ctx).First(&backup, id).Error
	return &backup, err
}

// GetAll 获取所有备份记录列表
func (r *BackupRepository) GetAll(ctx context.Context, page, pageSize int) ([]*model.Backup, int64, error) {
	var backups []*model.Backup
	var total int64

	db := r.db.WithContext(ctx)
	if err := db.Model(&model.Backup{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := db.Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&backups).Error

	return backups, total, err
}

// Update 更新备份记录
func (r *BackupRepository) Update(ctx context.Context, backup *model.Backup) error {
	return r.db.WithContext(ctx).Save(backup).Error
}

// Delete 删除备份记录
func (r *BackupRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.Backup{}, id).Error
}

// GetRecentBackups 获取最近的备份记录
func (r *BackupRepository) GetRecentBackups(ctx context.Context, limit int) ([]*model.Backup, error) {
	var backups []*model.Backup
	err := r.db.WithContext(ctx).Where("status = ?", string(model.BackupStatusCompleted)).
		Order("created_at DESC").
		Limit(limit).
		Find(&backups).Error
	return backups, err
}

// CleanupOldBackups 清理旧的备份记录（保留最近 30 天）
func (r *BackupRepository) CleanupOldBackups(ctx context.Context) error {
	threshold := time.Now().AddDate(0, 0, -30)
	return r.db.WithContext(ctx).Where("created_at < ?", threshold).
		Delete(&model.Backup{}).Error
}

// RestoreRecordRepository 恢复记录仓库
//
// 2026-07-22 方向E：所有方法第一参数改为 ctx context.Context。
type RestoreRecordRepository struct {
	db *gorm.DB
}

// NewRestoreRecordRepository 创建恢复记录仓库实例
func NewRestoreRecordRepository() *RestoreRecordRepository {
	return &RestoreRecordRepository{
		db: _db.GetDB(),
	}
}

// NewRestoreRecordRepositoryWithDB 使用指定数据库创建恢复记录仓库实例（用于测试）
func NewRestoreRecordRepositoryWithDB(database *gorm.DB) *RestoreRecordRepository {
	return &RestoreRecordRepository{
		db: database,
	}
}

// Create 创建恢复记录
func (r *RestoreRecordRepository) Create(ctx context.Context, record *model.RestoreRecord) error {
	return r.db.WithContext(ctx).Create(record).Error
}

// GetByID 根据 ID 获取恢复记录
func (r *RestoreRecordRepository) GetByID(ctx context.Context, id uint) (*model.RestoreRecord, error) {
	var record model.RestoreRecord
	err := r.db.WithContext(ctx).First(&record, id).Error
	return &record, err
}

// GetAll 获取所有恢复记录列表
func (r *RestoreRecordRepository) GetAll(ctx context.Context, page, pageSize int) ([]*model.RestoreRecord, int64, error) {
	var records []*model.RestoreRecord
	var total int64

	db := r.db.WithContext(ctx)
	if err := db.Model(&model.RestoreRecord{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := db.Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&records).Error

	return records, total, err
}

// Update 更新恢复记录
func (r *RestoreRecordRepository) Update(ctx context.Context, record *model.RestoreRecord) error {
	return r.db.WithContext(ctx).Save(record).Error
}

// GetLastRestore 获取最近一次恢复记录
func (r *RestoreRecordRepository) GetLastRestore(ctx context.Context) (*model.RestoreRecord, error) {
	var record model.RestoreRecord
	err := r.db.WithContext(ctx).Order("created_at DESC").
		First(&record).Error
	return &record, err
}
