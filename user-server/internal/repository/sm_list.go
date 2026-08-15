package repository

import (
	"context"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
	"time"

	"gorm.io/gorm"
)

type SmlistRepository interface {
	Create(ctx context.Context, user *model.Smlist) error
	GetByID(ctx context.Context, id string) (*model.Smlist, error)
	GetSmlistList(ctx context.Context, page int, limit int) ([]*model.Smlist, int64, error)
	GetSmlistAllList(ctx context.Context) ([]*model.Smlist, int64, error)
	Delete(ctx context.Context, id string) error
	GetRecentSmlistList(ctx context.Context) ([]*model.Smlist, error)
}

type smlistRepo struct {
	db *gorm.DB
}

func NewSmlistRepository(db ...*gorm.DB) SmlistRepository {
	if len(db) > 0 {
		return &smlistRepo{db: db[0]}
	}
	return &smlistRepo{db: _db.GetDB()}
}

func (r *smlistRepo) Create(ctx context.Context, smlist *model.Smlist) error {
	return r.db.Create(smlist).Error
}

func (r *smlistRepo) GetByID(ctx context.Context, id string) (*model.Smlist, error) {
	var smlist model.Smlist
	err := r.db.Where("id = ?", id).First(&smlist).Error
	return &smlist, err
}

func (r *smlistRepo) GetSmlistList(ctx context.Context, page int, limit int) ([]*model.Smlist, int64, error) {
	var smlists []*model.Smlist
	var total int64
	r.db.Model(&model.Smlist{}).Count(&total)
	err := r.db.Offset((page - 1) * limit).Limit(limit).Find(&smlists).Error
	return smlists, total, err
}

func (r *smlistRepo) GetSmlistAllList(ctx context.Context) ([]*model.Smlist, int64, error) {
	var smlists []*model.Smlist
	var total int64
	r.db.Model(&model.Smlist{}).Count(&total)
	err := r.db.Find(&smlists).Error
	return smlists, total, err
}

func (r *smlistRepo) Delete(ctx context.Context, id string) error {
	return r.db.Where("id = ?", id).Delete(&model.Smlist{}).Error
}

func (r *smlistRepo) GetRecentSmlistList(ctx context.Context) ([]*model.Smlist, error) {
	var smlists []*model.Smlist
	startTime := time.Now().Add(-time.Hour * 48)
	err := r.db.Where("created_at > ?", startTime).Order("created_at desc").Find(&smlists).Error
	return smlists, err
}

