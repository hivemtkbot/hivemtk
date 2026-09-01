package repository

import (
	"encoding/json"
	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// GeoKeywordGroupRepository GEO 关键词分组（话题聚类结果）
type GeoKeywordGroupRepository interface {
	// UpsertByName 按分组名 upsert（同品牌聚类覆盖更新）
	UpsertByName(name string, keywords []string) (*model.GeoKeywordGroup, error)
	GetByID(id string) (*model.GeoKeywordGroup, error)
	List() ([]*model.GeoKeywordGroup, error)
	Delete(id string) error
}

type geoKeywordGroupRepo struct {
	db *gorm.DB
}

// NewGeoKeywordGroupRepository 构造 repository（使用默认 DB）
func NewGeoKeywordGroupRepository() GeoKeywordGroupRepository {
	return &geoKeywordGroupRepo{db: _db.GetDB()}
}

// NewGeoKeywordGroupRepositoryWithDB 构造 repository（使用指定 DB，用于路由注入和测试）
func NewGeoKeywordGroupRepositoryWithDB(db *gorm.DB) GeoKeywordGroupRepository {
	return &geoKeywordGroupRepo{db: db}
}

func (r *geoKeywordGroupRepo) UpsertByName(name string, keywords []string) (*model.GeoKeywordGroup, error) {
	kwJSON, _ := json.Marshal(keywords)
	kwStr := string(kwJSON)

	var existing model.GeoKeywordGroup
	err := r.db.Where("name = ?", name).First(&existing).Error
	if err == nil {
		existing.KeywordList = kwStr
		existing.KeywordCount = len(keywords)
		if uErr := r.db.Save(&existing).Error; uErr != nil {
			return nil, uErr
		}
		return &existing, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}
	g := &model.GeoKeywordGroup{
		Name:         name,
		KeywordList:  kwStr,
		KeywordCount: len(keywords),
	}
	if cErr := r.db.Create(g).Error; cErr != nil {
		return nil, cErr
	}
	return g, nil
}

func (r *geoKeywordGroupRepo) GetByID(id string) (*model.GeoKeywordGroup, error) {
	var g model.GeoKeywordGroup
	if err := r.db.Where("id = ?", id).First(&g).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

func (r *geoKeywordGroupRepo) List() ([]*model.GeoKeywordGroup, error) {
	var groups []*model.GeoKeywordGroup
	if err := r.db.Order("updated_at DESC").Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func (r *geoKeywordGroupRepo) Delete(id string) error {
	return r.db.Delete(&model.GeoKeywordGroup{}, "id = ?", id).Error
}
