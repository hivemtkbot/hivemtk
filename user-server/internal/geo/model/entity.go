package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// GeoEntity GEO 实体表（v3 实体抽取 G7.1）
//
// 从 GeoKnowledgeDocument 中抽取的结构化实体（产品/人物/组织/地点/概念）。
// 由 EntityExtractorService 通过 LLM 批量抽取写入。
type GeoEntity struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"column:name;size:256;index" json:"name"`
	Type        string         `gorm:"column:type;size:64;index" json:"type"`
	Aliases     datatypes.JSON `gorm:"column:aliases;type:jsonb" json:"aliases"`
	Description string         `gorm:"column:description;type:text" json:"description"`
	SourceDocID uint           `gorm:"column:source_doc_id;index" json:"source_doc_id"`
	Confidence  float64        `gorm:"column:confidence;default:0.8" json:"confidence"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (GeoEntity) TableName() string { return "geo_entities" }

// GeoEntityRelation 实体关系表（v3 实体抽取 G7.3）
//
// 记录两个实体之间的关系（is_a/used_for/competitor_of/part_of 等），
// 为后续知识图谱和竞品对比分析提供结构化输入。
type GeoEntityRelation struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	EntityAID uint           `gorm:"column:entity_a_id;index" json:"entity_a_id"`
	EntityBID uint           `gorm:"column:entity_b_id;index" json:"entity_b_id"`
	Relation  string         `gorm:"column:relation;size:64;index" json:"relation"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (GeoEntityRelation) TableName() string { return "geo_entity_relations" }
