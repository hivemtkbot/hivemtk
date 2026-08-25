package repository

import (
	"errors"

	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// GeoConfigRepository GEO 全局配置仓储接口（单例：固定首行）
type GeoConfigRepository interface {
	Get() (*model.GeoConfig, error)
	Update(config *model.GeoConfig) error
}

type geoConfigRepo struct {
	db *gorm.DB
}

func NewGeoConfigRepository() GeoConfigRepository {
	return &geoConfigRepo{db: _db.GetDB()}
}

// NewGeoConfigRepositoryWithDB 创建指定数据库连接的实例（用于测试）
func NewGeoConfigRepositoryWithDB(db *gorm.DB) GeoConfigRepository {
	return &geoConfigRepo{db: db}
}

func (r *geoConfigRepo) Get() (*model.GeoConfig, error) {
	var config model.GeoConfig
	err := r.db.First(&config).Error
	return &config, err
}

func (r *geoConfigRepo) Update(config *model.GeoConfig) error {
	var existing model.GeoConfig
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

// GeoPlatformAccountRepository GEO 平台账号仓储接口
type GeoPlatformAccountRepository interface {
	Create(account *model.GeoPlatformAccount) error
	GetByID(id string) (*model.GeoPlatformAccount, error)
	GetByPlatformAndName(platform, accountName string) (*model.GeoPlatformAccount, error)
	GetLatestByPlatform(platform string) (*model.GeoPlatformAccount, error)
	GetList(platform string, page, limit int) ([]*model.GeoPlatformAccount, int64, error)
	Delete(id string) error
	Update(account *model.GeoPlatformAccount) error
}

type geoPlatformAccountRepo struct {
	db *gorm.DB
}

func NewGeoPlatformAccountRepository() GeoPlatformAccountRepository {
	return &geoPlatformAccountRepo{db: _db.GetDB()}
}

// NewGeoPlatformAccountRepositoryWithDB 创建指定数据库连接的实例（用于测试）
func NewGeoPlatformAccountRepositoryWithDB(db *gorm.DB) GeoPlatformAccountRepository {
	return &geoPlatformAccountRepo{db: db}
}

func (r *geoPlatformAccountRepo) Create(account *model.GeoPlatformAccount) error {
	return r.db.Create(account).Error
}

func (r *geoPlatformAccountRepo) GetByID(id string) (*model.GeoPlatformAccount, error) {
	var account model.GeoPlatformAccount
	err := r.db.Where("id = ?", id).First(&account).Error
	return &account, err
}

func (r *geoPlatformAccountRepo) GetByPlatformAndName(platform, accountName string) (*model.GeoPlatformAccount, error) {
	var account model.GeoPlatformAccount
	err := r.db.Where("platform = ? AND account_name = ?", platform, accountName).First(&account).Error
	return &account, err
}

// GetLatestByPlatform 返回指定平台最近更新的账号（发布时优先取用其存储凭据）。
// 以 updated_at 为准：SaveAccount 的覆盖更新不刷新 created_at，
// 旧账号更新凭据后仍应被优先取用；id 兜底保证同刻并列时结果确定。
func (r *geoPlatformAccountRepo) GetLatestByPlatform(platform string) (*model.GeoPlatformAccount, error) {
	var account model.GeoPlatformAccount
	err := r.db.Where("platform = ?", platform).
		Order("updated_at DESC, created_at DESC, id DESC").
		First(&account).Error
	return &account, err
}

func (r *geoPlatformAccountRepo) GetList(platform string, page, limit int) ([]*model.GeoPlatformAccount, int64, error) {
	var accounts []*model.GeoPlatformAccount
	var total int64
	offset := (page - 1) * limit

	query := r.db.Model(&model.GeoPlatformAccount{})

	if platform != "" {
		query = query.Where("platform = ?", platform)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&accounts).Error
	return accounts, total, err
}

func (r *geoPlatformAccountRepo) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.GeoPlatformAccount{}).Error
}

func (r *geoPlatformAccountRepo) Update(account *model.GeoPlatformAccount) error {
	return r.db.Save(account).Error
}

// GeoPublishRecordRepository GEO 发布记录仓储接口
type GeoPublishRecordRepository interface {
	Create(record *model.GeoPublishRecord) error
	GetByID(id string) (*model.GeoPublishRecord, error)
	GetList(articleID, platform string, page, limit int) ([]*model.GeoPublishRecord, int64, error)
	Update(record *model.GeoPublishRecord) error
}

type geoPublishRecordRepo struct {
	db *gorm.DB
}

func NewGeoPublishRecordRepository() GeoPublishRecordRepository {
	return &geoPublishRecordRepo{db: _db.GetDB()}
}

// NewGeoPublishRecordRepositoryWithDB 创建指定数据库连接的实例（用于测试）
func NewGeoPublishRecordRepositoryWithDB(db *gorm.DB) GeoPublishRecordRepository {
	return &geoPublishRecordRepo{db: db}
}

func (r *geoPublishRecordRepo) Create(record *model.GeoPublishRecord) error {
	return r.db.Create(record).Error
}

func (r *geoPublishRecordRepo) GetByID(id string) (*model.GeoPublishRecord, error) {
	var record model.GeoPublishRecord
	err := r.db.Where("id = ?", id).First(&record).Error
	return &record, err
}

func (r *geoPublishRecordRepo) GetList(articleID, platform string, page, limit int) ([]*model.GeoPublishRecord, int64, error) {
	var records []*model.GeoPublishRecord
	var total int64
	offset := (page - 1) * limit

	query := r.db.Model(&model.GeoPublishRecord{})

	if articleID != "" {
		query = query.Where("article_id = ?", articleID)
	}
	if platform != "" {
		query = query.Where("platform = ?", platform)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&records).Error
	return records, total, err
}

func (r *geoPublishRecordRepo) Update(record *model.GeoPublishRecord) error {
	return r.db.Save(record).Error
}
