package repository

// sop_template.go SOP 模板 Repository
//
// 五层架构归属: L5 数据访问层
// 设计依据: 2026-07-31 AI 智能体性能优化 (T3)
//   - Layer1 路由依赖 SOP 模板快速匹配
//   - 当 FAQ 未命中 + SOP 高置信 -> 模板拼接回复 (<50ms, 零 LLM)
//   - 支持按 (intent, stage) 查询 + 按 priority/confidence 排序
//
// 方法:
//   - Create           新增 SOP 模板
//   - GetByID          按 ID 查询
//   - ListEnabled      查询所有启用的模板
//   - MatchByIntent    按意图匹配 (返回最高优先级)
//   - MatchByIntentStage 按 (intent, stage) 匹配
//   - IncrementHitCount 命中次数 +1

import (
	"context"

	"marketing/internal/model"

	"gorm.io/gorm"
)

type SOPTemplateRepository struct {
	db *gorm.DB
}

func NewSOPTemplateRepository(db *gorm.DB) *SOPTemplateRepository {
	return &SOPTemplateRepository{db: db}
}

// Create 新增 SOP 模板
func (r *SOPTemplateRepository) Create(ctx context.Context, tpl *model.SOPTemplate) error {
	return r.db.WithContext(ctx).Select("*").Create(tpl).Error
}

// GetByID 按 ID 查询
func (r *SOPTemplateRepository) GetByID(ctx context.Context, id uint) (*model.SOPTemplate, error) {
	var tpl model.SOPTemplate
	if err := r.db.WithContext(ctx).First(&tpl, id).Error; err != nil {
		return nil, err
	}
	return &tpl, nil
}

// ListEnabled 查询所有启用的 SOP 模板 (按 priority DESC 排序)
func (r *SOPTemplateRepository) ListEnabled(ctx context.Context, limit int) ([]model.SOPTemplate, error) {
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}
	var tpls []model.SOPTemplate
	err := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("priority DESC, confidence DESC, id ASC").
		Limit(limit).
		Find(&tpls).Error
	return tpls, err
}

// MatchByIntent 按意图匹配 SOP 模板 (取最高优先级且启用)
func (r *SOPTemplateRepository) MatchByIntent(ctx context.Context, intent string) ([]model.SOPTemplate, error) {
	if intent == "" {
		return nil, nil
	}
	var tpls []model.SOPTemplate
	err := r.db.WithContext(ctx).
		Where("intent = ? AND enabled = ?", intent, true).
		Order("priority DESC, confidence DESC, id ASC").
		Limit(10).
		Find(&tpls).Error
	return tpls, err
}

// MatchByIntentStage 按 (intent, stage) 精确匹配 SOP 模板
func (r *SOPTemplateRepository) MatchByIntentStage(ctx context.Context, intent, stage string) ([]model.SOPTemplate, error) {
	if intent == "" || stage == "" {
		return nil, nil
	}
	var tpls []model.SOPTemplate
	err := r.db.WithContext(ctx).
		Where("intent = ? AND stage = ? AND enabled = ?", intent, stage, true).
		Order("priority DESC, confidence DESC, id ASC").
		Limit(10).
		Find(&tpls).Error
	return tpls, err
}

// IncrementHitCount 命中次数 +1
func (r *SOPTemplateRepository) IncrementHitCount(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).
		Model(&model.SOPTemplate{}).
		Where("id = ?", id).
		UpdateColumn("hit_count", gorm.Expr("hit_count + 1")).
		Error
}
