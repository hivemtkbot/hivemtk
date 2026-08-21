package repository

import (
	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// GeoKnowledgeDocumentRepository GEO 知识库文档仓储接口
type GeoKnowledgeDocumentRepository interface {
	Create(doc *model.GeoKnowledgeDocument) error
	Update(doc *model.GeoKnowledgeDocument) error
	GetByID(id string) (*model.GeoKnowledgeDocument, error)
	GetList() ([]*model.GeoKnowledgeDocument, error)
	Delete(id string) error
}

type geoKnowledgeDocRepo struct {
	db *gorm.DB
}

func NewGeoKnowledgeDocumentRepository() GeoKnowledgeDocumentRepository {
	return &geoKnowledgeDocRepo{db: _db.GetDB()}
}

// NewGeoKnowledgeDocumentRepositoryWithDB 创建指定数据库连接的实例（用于测试）
func NewGeoKnowledgeDocumentRepositoryWithDB(db *gorm.DB) GeoKnowledgeDocumentRepository {
	return &geoKnowledgeDocRepo{db: db}
}

func (r *geoKnowledgeDocRepo) Create(doc *model.GeoKnowledgeDocument) error {
	return r.db.Create(doc).Error
}

func (r *geoKnowledgeDocRepo) Update(doc *model.GeoKnowledgeDocument) error {
	return r.db.Save(doc).Error
}

func (r *geoKnowledgeDocRepo) GetByID(id string) (*model.GeoKnowledgeDocument, error) {
	var doc model.GeoKnowledgeDocument
	err := r.db.Where("id = ?", id).First(&doc).Error
	return &doc, err
}

func (r *geoKnowledgeDocRepo) GetList() ([]*model.GeoKnowledgeDocument, error) {
	var docs []*model.GeoKnowledgeDocument
	err := r.db.Order("created_at DESC").Find(&docs).Error
	return docs, err
}

func (r *geoKnowledgeDocRepo) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.GeoKnowledgeDocument{}).Error
}
