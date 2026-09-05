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

func (r *emailListRepo) Create(ctx context.Context, list *model.EmailList) error {
	return r.db.WithContext(ctx).Create(list).Error
}

func (r *emailListRepo) BatchCreate(ctx context.Context, list []*model.EmailList) (int64, error) {
	result := r.db.WithContext(ctx).CreateInBatches(list, 100)
	return result.RowsAffected, nil
}

func (r *emailListRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.EmailList, error) {
	var list model.EmailList
	if err := r.db.WithContext(ctx).First(&list, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &list, nil
}

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

func (r *emailListRepo) Update(ctx context.Context, list *model.EmailList) error {
	return r.db.WithContext(ctx).Save(list).Error
}

func (r *emailListRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.EmailList{}, "id = ?", id).Error
}

func (r *emailListRepo) GetUnsentEmailList(ctx context.Context, limit int) ([]*model.EmailList, error) {
	var emailLists []*model.EmailList
	err := r.db.WithContext(ctx).Where("is_send = ?", 0).Order("created_at ASC").Limit(limit).Find(&emailLists).Error
	if err != nil {
		return nil, err
	}
	return emailLists, nil
}

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

func (r *emailListRepo) GetByTraceID(ctx context.Context, traceID uuid.UUID) (*model.EmailList, error) {
	var emailList model.EmailList
	err := r.db.WithContext(ctx).First(&emailList, "trace_id = ?", traceID).Error
	if err != nil {
		return nil, err
	}
	return &emailList, nil
}
