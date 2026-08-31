package repository

import (
	"context"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EmailListRepository 列表仓库接口
type EmailListRepository interface {
	Create(ctx context.Context, list *model.EmailList) error
	BatchCreate(ctx context.Context, list []*model.EmailList) (int64, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.EmailList, error)
	List(ctx context.Context, page int, pageSize int) ([]*model.EmailList, int64, error)
	Update(ctx context.Context, list *model.EmailList) error
	Delete(ctx context.Context, id uuid.UUID) error
	GetUnsentEmailList(ctx context.Context, limit int) ([]*model.EmailList, error)
	GetTodayCountByFrom(ctx context.Context, from string) (int64, error)
	GetByTraceID(ctx context.Context, traceID uuid.UUID) (*model.EmailList, error)
}

type emailListRepo struct {
	db *gorm.DB
}

// NewEmailListRepository 创建列表仓库实例
func NewEmailListRepository() EmailListRepository {
	return &emailListRepo{db: _db.GetDB()}
}

// Create 创建列表
func (r *emailListRepo) Create(ctx context.Context, list *model.EmailList) error {
	return r.db.WithContext(ctx).Create(list).Error
}

func (r *emailListRepo) BatchCreate(ctx context.Context, list []*model.EmailList) (int64, error) {
	result := r.db.WithContext(ctx).CreateInBatches(list, 100)
	return result.RowsAffected, nil
}

// GetByID 根据ID获取列表
func (r *emailListRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.EmailList, error) {
	var list model.EmailList
	if err := r.db.WithContext(ctx).First(&list, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &list, nil
}

// List 获取所有列表
func (r *emailListRepo) List(ctx context.Context, page int, pageSize int) ([]*model.EmailList, int64, error) {
	var emailLists []*model.EmailList
	var total int64
	db := r.db.WithContext(ctx)
	err := db.Model(&model.EmailList{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = db.Offset((page - 1) * pageSize).Limit(pageSize).Order("created_at DESC").Find(&emailLists).Error
	if err != nil {
		return nil, 0, err
	}
	return emailLists, total, err
}

// Update 更新列表
func (r *emailListRepo) Update(ctx context.Context, list *model.EmailList) error {
	return r.db.WithContext(ctx).Save(list).Error
}

// Delete 删除列表
func (r *emailListRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.EmailList{}, "id = ?", id).Error
}

// GetUnsentEmailList 获取未发送的邮件列表
func (r *emailListRepo) GetUnsentEmailList(ctx context.Context, limit int) ([]*model.EmailList, error) {
	var emailLists []*model.EmailList
	err := r.db.WithContext(ctx).Where("is_send = ?", 0).Order("created_at ASC").Limit(limit).Find(&emailLists).Error
	if err != nil {
		return nil, err
	}
	return emailLists, nil
}

// GetTodayCountByFrom 获取今日发送个数
// 注："from" 是 SQL 关键字，PostgreSQL 中需用双引号包裹；GORM 的 ? 占位符无法处理列名，需直接字符串拼接（from 参数来自上层调用，非用户输入）。
func (r *emailListRepo) GetTodayCountByFrom(ctx context.Context, from string) (int64, error) {
	var count int64
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 1, 0, now.Location())
	end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())

	err := r.db.WithContext(ctx).Model(&model.EmailList{}).
		Where("\"from\" = ? AND send_time >= ? AND send_time < ?", from, start, end).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

// GetByTraceID 根据trace id 获取邮件信息
func (r *emailListRepo) GetByTraceID(ctx context.Context, traceID uuid.UUID) (*model.EmailList, error) {
	var emailList model.EmailList
	err := r.db.WithContext(ctx).First(&emailList, "trace_id = ?", traceID).Error
	if err != nil {
		return nil, err
	}
	return &emailList, nil
}
