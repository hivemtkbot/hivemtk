package repository

import (
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EmailJobsRepository 任务仓库接口
type EmailJobsRepository interface {
	Create(jobs *model.EmailJobs) error
	GetByID(id uuid.UUID) (*model.EmailJobs, error)
	List(page int, pageSize int) ([]*model.EmailJobs, int64, error)
	Update(jobs *model.EmailJobs) error
	Delete(id uuid.UUID) error
}

type emailJobsRepo struct {
	db *gorm.DB
}

// NewEmailJobsRepository 创建任务仓库实例
func NewEmailJobsRepository() EmailJobsRepository {
	return &emailJobsRepo{db: _db.GetDB()}
}

// Create 创建任务
func (r *emailJobsRepo) Create(jobs *model.EmailJobs) error {
	return r.db.Create(jobs).Error
}

// GetByID 根据ID获取任务
func (r *emailJobsRepo) GetByID(id uuid.UUID) (*model.EmailJobs, error) {
	var jobs model.EmailJobs
	if err := r.db.First(&jobs, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &jobs, nil
}

// List 获取所有任务
func (r *emailJobsRepo) List(page int, pageSize int) ([]*model.EmailJobs, int64, error) {
	var jobsLists []*model.EmailJobs
	var total int64
	// 分别查询list 和 total
	err := r.db.Model(&model.EmailJobs{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = r.db.Offset((page - 1) * pageSize).Limit(pageSize).Order("created_at DESC").Find(&jobsLists).Error
	if err != nil {
		return nil, 0, err
	}
	return jobsLists, total, err
}

// Update 更新任务
func (r *emailJobsRepo) Update(jobs *model.EmailJobs) error {
	return r.db.Save(jobs).Error
}

// Delete 删除任务
func (r *emailJobsRepo) Delete(id uuid.UUID) error {
	return r.db.Delete(&model.EmailJobs{}, "id = ?", id).Error
}
