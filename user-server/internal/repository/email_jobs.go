package repository

import (
	"context"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EmailJobsRepository 任务仓库接口
type EmailJobsRepository interface {
	Create(ctx context.Context, jobs *model.EmailJobs) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.EmailJobs, error)
	List(ctx context.Context, page int, pageSize int) ([]*model.EmailJobs, int64, error)
	Update(ctx context.Context, jobs *model.EmailJobs) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type emailJobsRepo struct {
	db *gorm.DB
}

// NewEmailJobsRepository 创建任务仓库实例
func NewEmailJobsRepository() EmailJobsRepository {
	return &emailJobsRepo{db: _db.GetDB()}
}

// Create 创建任务
func (r *emailJobsRepo) Create(ctx context.Context, jobs *model.EmailJobs) error {
	return r.db.WithContext(ctx).Create(jobs).Error
}

// GetByID 根据ID获取任务
func (r *emailJobsRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.EmailJobs, error) {
	var jobs model.EmailJobs
	if err := r.db.WithContext(ctx).First(&jobs, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &jobs, nil
}

// List 获取所有任务
func (r *emailJobsRepo) List(ctx context.Context, page int, pageSize int) ([]*model.EmailJobs, int64, error) {
	var jobsLists []*model.EmailJobs
	var total int64
	db := r.db.WithContext(ctx)
	// 分别查询list 和 total
	err := db.Model(&model.EmailJobs{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = db.Offset((page - 1) * pageSize).Limit(pageSize).Order("created_at DESC").Find(&jobsLists).Error
	if err != nil {
		return nil, 0, err
	}
	return jobsLists, total, err
}

// Update 更新任务
func (r *emailJobsRepo) Update(ctx context.Context, jobs *model.EmailJobs) error {
	return r.db.WithContext(ctx).Save(jobs).Error
}

// Delete 删除任务
func (r *emailJobsRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.EmailJobs{}, "id = ?", id).Error
}
