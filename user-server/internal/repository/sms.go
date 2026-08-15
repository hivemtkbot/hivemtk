package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// SmsRepository 短信仓库接口
type SmsRepository interface {
	GetConfig(ctx context.Context) (*model.SmsConfig, error)
	SaveConfig(ctx context.Context, config *model.SmsConfig) error
	GetAliyunConfig(ctx context.Context) (*model.SmsAliyunConfig, error)
	SaveAliyunConfig(ctx context.Context, config *model.SmsAliyunConfig) error
	GetTencentConfig(ctx context.Context) (*model.SmsTencentConfig, error)
	SaveTencentConfig(ctx context.Context, config *model.SmsTencentConfig) error
	GetHuaweiConfig(ctx context.Context) (*model.SmsHuaweiConfig, error)
	SaveHuaweiConfig(ctx context.Context, config *model.SmsHuaweiConfig) error

	GetSmsList(ctx context.Context, page, limit int, phone, status, startDate, endDate string) ([]*model.SmsRecord, int64, error)
	GetSmsByID(ctx context.Context, id uint) (*model.SmsRecord, error)
	CreateSmsRecord(ctx context.Context, record *model.SmsRecord) error
	UpdateSmsRecord(ctx context.Context, record *model.SmsRecord) error

	GetDraftList(ctx context.Context, page, limit int, title string) ([]*model.SmsDraft, int64, error)
	GetDraftByID(ctx context.Context, id uint) (*model.SmsDraft, error)
	CreateDraft(ctx context.Context, draft *model.SmsDraft) error
	UpdateDraft(ctx context.Context, draft *model.SmsDraft) error
	DeleteDraft(ctx context.Context, id uint) error

	GetJobList(ctx context.Context, page, limit int, status, name string) ([]*model.SmsJob, int64, error)
	GetJobByID(ctx context.Context, id uint) (*model.SmsJob, error)
	CreateJob(ctx context.Context, job *model.SmsJob) error
	UpdateJob(ctx context.Context, job *model.SmsJob) error
	DeleteJob(ctx context.Context, id uint) error
	CreateJobDetails(ctx context.Context, details []*model.SmsJobDetail) error
	GetJobDetails(ctx context.Context, jobID uint, page, limit int) ([]*model.SmsJobDetail, int64, error)
	DeleteJobDetails(ctx context.Context, jobID uint) error
}

// smsRepository 短信仓库实现
type smsRepository struct {
	db *gorm.DB
}

// NewSmsRepository 创建短信仓库
//
// 五层架构约定：Repository 自身负责获取 db 连接，调用方无需注入 *gorm.DB。
// 这样 Service 层不会因传递 db 而反向依赖 db 包。
func NewSmsRepository() SmsRepository {
	return &smsRepository{db: db.GetDB()}
}

// GetConfig 获取短信配置
func (r *smsRepository) GetConfig(ctx context.Context) (*model.SmsConfig, error) {
	var config model.SmsConfig
	err := r.db.First(&config).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &model.SmsConfig{
				DefaultProvider: "aliyun",
				RateLimit:       100,
				DailyLimit:      10000,
				RetryTimes:      3,
			}, nil
		}
		return nil, err
	}
	return &config, nil
}

// SaveConfig 保存短信配置
func (r *smsRepository) SaveConfig(ctx context.Context, config *model.SmsConfig) error {
	var existing model.SmsConfig
	err := r.db.First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return r.db.Create(config).Error
		}
		return err
	}
	config.ID = existing.ID
	return r.db.Save(config).Error
}

// GetAliyunConfig 获取阿里云配置
func (r *smsRepository) GetAliyunConfig(ctx context.Context) (*model.SmsAliyunConfig, error) {
	var config model.SmsAliyunConfig
	err := r.db.First(&config).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &model.SmsAliyunConfig{}, nil
		}
		return nil, err
	}
	return &config, nil
}

// SaveAliyunConfig 保存阿里云配置
func (r *smsRepository) SaveAliyunConfig(ctx context.Context, config *model.SmsAliyunConfig) error {
	var existing model.SmsAliyunConfig
	err := r.db.First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return r.db.Create(config).Error
		}
		return err
	}
	config.ID = existing.ID
	return r.db.Save(config).Error
}

// GetTencentConfig 获取腾讯云配置
func (r *smsRepository) GetTencentConfig(ctx context.Context) (*model.SmsTencentConfig, error) {
	var config model.SmsTencentConfig
	err := r.db.First(&config).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &model.SmsTencentConfig{}, nil
		}
		return nil, err
	}
	return &config, nil
}

// SaveTencentConfig 保存腾讯云配置
func (r *smsRepository) SaveTencentConfig(ctx context.Context, config *model.SmsTencentConfig) error {
	var existing model.SmsTencentConfig
	err := r.db.First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return r.db.Create(config).Error
		}
		return err
	}
	config.ID = existing.ID
	return r.db.Save(config).Error
}

// GetHuaweiConfig 获取华为云配置
func (r *smsRepository) GetHuaweiConfig(ctx context.Context) (*model.SmsHuaweiConfig, error) {
	var config model.SmsHuaweiConfig
	err := r.db.First(&config).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &model.SmsHuaweiConfig{}, nil
		}
		return nil, err
	}
	return &config, nil
}

// SaveHuaweiConfig 保存华为云配置
func (r *smsRepository) SaveHuaweiConfig(ctx context.Context, config *model.SmsHuaweiConfig) error {
	var existing model.SmsHuaweiConfig
	err := r.db.First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return r.db.Create(config).Error
		}
		return err
	}
	config.ID = existing.ID
	return r.db.Save(config).Error
}

// GetSmsList 获取短信列表
func (r *smsRepository) GetSmsList(ctx context.Context, page, limit int, phone, status, startDate, endDate string) ([]*model.SmsRecord, int64, error) {
	var records []*model.SmsRecord
	var total int64

	query := r.db.Model(&model.SmsRecord{})

	if phone != "" {
		query = query.Where("phone = ?", phone)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if startDate != "" {
		query = query.Where("created_at >= ?", startDate)
	}
	if endDate != "" {
		endDate = endDate + " 23:59:59"
		query = query.Where("created_at <= ?", endDate)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&records).Error; err != nil {
		return nil, 0, err
	}

	return records, total, nil
}

// GetSmsByID 根据ID获取短信
func (r *smsRepository) GetSmsByID(ctx context.Context, id uint) (*model.SmsRecord, error) {
	var record model.SmsRecord
	err := r.db.First(&record, id).Error
	return &record, err
}

// CreateSmsRecord 创建短信记录
func (r *smsRepository) CreateSmsRecord(ctx context.Context, record *model.SmsRecord) error {
	return r.db.Create(record).Error
}

// UpdateSmsRecord 更新短信记录
func (r *smsRepository) UpdateSmsRecord(ctx context.Context, record *model.SmsRecord) error {
	return r.db.Save(record).Error
}

// GetDraftList 获取草稿列表
func (r *smsRepository) GetDraftList(ctx context.Context, page, limit int, title string) ([]*model.SmsDraft, int64, error) {
	var drafts []*model.SmsDraft
	var total int64

	query := r.db.Model(&model.SmsDraft{})

	if title != "" {
		query = query.Where("title LIKE ?", "%"+title+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("updated_at DESC").Offset(offset).Limit(limit).Find(&drafts).Error; err != nil {
		return nil, 0, err
	}

	return drafts, total, nil
}

// GetDraftByID 根据ID获取草稿
func (r *smsRepository) GetDraftByID(ctx context.Context, id uint) (*model.SmsDraft, error) {
	var draft model.SmsDraft
	err := r.db.First(&draft, id).Error
	return &draft, err
}

// CreateDraft 创建草稿
func (r *smsRepository) CreateDraft(ctx context.Context, draft *model.SmsDraft) error {
	return r.db.Create(draft).Error
}

// UpdateDraft 更新草稿
func (r *smsRepository) UpdateDraft(ctx context.Context, draft *model.SmsDraft) error {
	return r.db.Save(draft).Error
}

// DeleteDraft 删除草稿
func (r *smsRepository) DeleteDraft(ctx context.Context, id uint) error {
	return r.db.Delete(&model.SmsDraft{}, id).Error
}

// GetJobList 获取任务列表
func (r *smsRepository) GetJobList(ctx context.Context, page, limit int, status, name string) ([]*model.SmsJob, int64, error) {
	var jobs []*model.SmsJob
	var total int64

	query := r.db.Model(&model.SmsJob{})

	if status != "" {
		query = query.Where("status = ?", status)
	}
	if name != "" {
		query = query.Where("name LIKE ?", "%"+name+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&jobs).Error; err != nil {
		return nil, 0, err
	}

	return jobs, total, nil
}

// GetJobByID 根据ID获取任务
func (r *smsRepository) GetJobByID(ctx context.Context, id uint) (*model.SmsJob, error) {
	var job model.SmsJob
	err := r.db.First(&job, id).Error
	return &job, err
}

// CreateJob 创建任务
func (r *smsRepository) CreateJob(ctx context.Context, job *model.SmsJob) error {
	return r.db.Create(job).Error
}

// UpdateJob 更新任务
func (r *smsRepository) UpdateJob(ctx context.Context, job *model.SmsJob) error {
	return r.db.Save(job).Error
}

// DeleteJob 删除任务
func (r *smsRepository) DeleteJob(ctx context.Context, id uint) error {
	return r.db.Delete(&model.SmsJob{}, id).Error
}

// DeleteJobDetails 删除任务详情
func (r *smsRepository) DeleteJobDetails(ctx context.Context, jobID uint) error {
	return r.db.Where("job_id = ?", jobID).Delete(&model.SmsJobDetail{}).Error
}

// CreateJobDetails 创建任务详情
func (r *smsRepository) CreateJobDetails(ctx context.Context, details []*model.SmsJobDetail) error {
	return r.db.CreateInBatches(details, 100).Error
}

// GetJobDetails 获取任务详情列表
func (r *smsRepository) GetJobDetails(ctx context.Context, jobID uint, page, limit int) ([]*model.SmsJobDetail, int64, error) {
	var details []*model.SmsJobDetail
	var total int64

	query := r.db.Model(&model.SmsJobDetail{}).Where("job_id = ?", jobID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("created_at ASC").Offset(offset).Limit(limit).Find(&details).Error; err != nil {
		return nil, 0, err
	}

	return details, total, nil
}

