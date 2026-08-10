package repository

// script_library_repository.go 话术库仓储
//
// 五层架构归属: L4 数据访问层
//
// 覆盖：ScriptLibrary 表的查询与使用统计更新，
// 服务于 ObjectionHandlerService（异议处理服务）。

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
)

// ScriptLibraryRepository 话术库仓储
type ScriptLibraryRepository struct {
	db *gorm.DB
}

// NewScriptLibraryRepository 创建话术库仓储
func NewScriptLibraryRepository(db *gorm.DB) *ScriptLibraryRepository {
	return &ScriptLibraryRepository{db: db}
}

// ListObjectionTemplates 查询异议处理模板
//
// 匹配规则：category=objection 或 subcategory=objectionCategory
// 排序：usage_count DESC
// 限制：limit 条（limit<=0 时不限制）
func (r *ScriptLibraryRepository) ListObjectionTemplates(ctx context.Context, objectionCategory string, limit int) ([]model.ScriptLibrary, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("script library repository not initialized")
	}
	q := r.db.WithContext(ctx).
		Where("category = ? OR subcategory = ?", "objection", objectionCategory).
		Order("usage_count DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var scripts []model.ScriptLibrary
	if err := q.Find(&scripts).Error; err != nil {
		return nil, err
	}
	return scripts, nil
}

// IncrementUsageStats 更新话术使用统计
//
//   - 始终增加 usage_count
//   - success=true 时同时增加 success_count，并按 success_count / GREATEST(usage_count, 1) 重算 conversion_rate
//   - success=false 时仅增加 usage_count
//
// 保留原 ObjectionHandlerService.RecordUsage 的 gorm.Expr 表达式以保持等价行为。
func (r *ScriptLibraryRepository) IncrementUsageStats(ctx context.Context, templateID uint, success bool) error {
	if r == nil || r.db == nil {
		return errors.New("script library repository not initialized")
	}
	updates := map[string]any{
		"usage_count": gorm.Expr("usage_count + 1"),
	}
	if success {
		updates["success_count"] = gorm.Expr("success_count + 1")
		updates["conversion_rate"] = gorm.Expr("success_count::float / GREATEST(usage_count, 1)")
	}
	return r.db.WithContext(ctx).
		Model(&model.ScriptLibrary{}).
		Where("id = ?", templateID).
		Updates(updates).Error
}
