package repository

import (
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
	"time"

	"gorm.io/gorm"
)

type SmlistRepository interface {
	Create(user *model.Smlist) error
	GetByID(id string) (*model.Smlist, error)
	GetSmlistList(page int, limit int) ([]*model.Smlist, int64, error)
	GetSmlistAllList() ([]*model.Smlist, int64, error)
	Delete(id string) error
	GetRecentSmlistList() ([]*model.Smlist, error)
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

func (r *smlistRepo) Create(smlist *model.Smlist) error {
	return r.db.Create(smlist).Error
}

func (r *smlistRepo) GetByID(id string) (*model.Smlist, error) {
	var smlist model.Smlist
	err := r.db.Where("id = ?", id).First(&smlist).Error
	return &smlist, err
}

func (r *smlistRepo) GetSmlistList(page int, limit int) ([]*model.Smlist, int64, error) {
	var smlists []*model.Smlist
	var total int64
	r.db.Model(&model.Smlist{}).Count(&total)
	err := r.db.Offset((page - 1) * limit).Limit(limit).Find(&smlists).Error
	return smlists, total, err
}

func (r *smlistRepo) GetSmlistAllList() ([]*model.Smlist, int64, error) {
	var smlists []*model.Smlist
	var total int64
	r.db.Model(&model.Smlist{}).Count(&total)
	err := r.db.Find(&smlists).Error
	return smlists, total, err
}

func (r *smlistRepo) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.Smlist{}).Error
}

func (r *smlistRepo) GetRecentSmlistList() ([]*model.Smlist, error) {
	var smlists []*model.Smlist
	// 最近 48 小时的数据
	startTime := time.Now().Add(-time.Hour * 48)
	err := r.db.Where("created_at > ?", startTime).Order("created_at desc").Find(&smlists).Error
	return smlists, err
}
