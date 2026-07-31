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

// MatchByIDs 按 ID 集合 + 意图 + 阶段匹配 (2026-07-31 P1-A: 智能体绑定 SOP 范围)
//
// 当 agent 绑定了 SOP template IDs 时, 仅在绑定 ID 集合内匹配;
//
// 绑定为空 = 走原 MatchByIntentStage 全局匹配
func (r *SOPTemplateRepository) MatchByIDs(ctx context.Context, intent, stage string, ids []string) ([]model.SOPTemplate, error) {
	if intent == "" || len(ids) == 0 {
		return nil, nil
	}
	var tpls []model.SOPTemplate
	q := r.db.WithContext(ctx).
		Where("intent = ? AND id IN ? AND enabled = ?", intent, ids, true)
	if stage != "" {
		q = q.Where("stage = ?", stage)
	}
	err := q.Order("priority DESC, confidence DESC, id ASC").
		Limit(10).
		Find(&tpls).Error
	return tpls, err
}

// ListWithFilter 分页+过滤 (前端 SOP 模板管理页面使用)
func (r *SOPTemplateRepository) ListWithFilter(ctx context.Context, filter SOPTemplateFilter) ([]model.SOPTemplate, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.SOPTemplate{})
	if filter.Keyword != "" {
		kw := "%" + filter.Keyword + "%"
		q = q.Where("name LIKE ? OR template LIKE ?", kw, kw)
	}
	if filter.Intent != "" {
		q = q.Where("intent = ?", filter.Intent)
	}
	if filter.Stage != "" {
		q = q.Where("stage = ?", filter.Stage)
	}
	if filter.Enabled != nil {
		q = q.Where("enabled = ?", *filter.Enabled)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var tpls []model.SOPTemplate
	offset := (filter.Page - 1) * filter.PageSize
	if offset < 0 {
		offset = 0
	}
	err := q.Order("priority DESC, confidence DESC, id ASC").
		Offset(offset).Limit(filter.PageSize).
		Find(&tpls).Error
	return tpls, total, err
}

// Update 更新 SOP 模板
func (r *SOPTemplateRepository) Update(ctx context.Context, id uint, tpl *model.SOPTemplate) error {
	return r.db.WithContext(ctx).Model(&model.SOPTemplate{}).
		Where("id = ?", id).
		Select("*").Updates(tpl).Error
}

// Delete 删除 SOP 模板
func (r *SOPTemplateRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.SOPTemplate{}).Error
}

// SOPTemplateFilter SOP 模板查询过滤器 (前端管理页面)
type SOPTemplateFilter struct {
	Keyword  string
	Intent   string
	Stage    string
	Enabled  *bool
	Page     int
	PageSize int
}
