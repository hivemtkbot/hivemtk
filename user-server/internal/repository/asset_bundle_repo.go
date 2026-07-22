// Package repository 提供 AssetBundle（资产包）的 CRUD 仓储实现。
//
// 方向9：资产包模式
// 文档依据：docs/企业级架构优化/资产包模式.md
//
// 设计原则：
//  1. 五层架构：Repository 只做 GORM 映射，不做业务校验（业务校验放 Service）
//  2. 软删除：使用 gorm.DeletedAt
//  3. 唯一约束：AssetID 业务唯一键
//  4. 索引：Status / Scope / Author / Industry / Language
package repository

import (
	"context"
	"errors"
	"strings"

	"marketing/internal/model"

	"gorm.io/gorm"
)

// AssetBundleFilter 列表过滤
type AssetBundleFilter struct {
	Keyword  string                       // 模糊查询 Title / Description
	Author   string                       // 精确作者
	Industry string                       // 行业
	Language string                       // 语言
	Scope    model.AssetBundleScope       // 作用域
	Status   model.AssetBundleStatus      // 状态
	Tags     []string                     // 至少命中一个 tag
	Page     int                          // 1-based
	Size     int                          // default 20
}

// AssetBundleRepository 资产包仓储接口
type AssetBundleRepository interface {
	Create(ctx context.Context, m *model.AssetBundle) error
	Update(ctx context.Context, m *model.AssetBundle) error
	Save(ctx context.Context, m *model.AssetBundle) error
	SoftDelete(ctx context.Context, id int64) error
	HardDelete(ctx context.Context, id int64) error
	FindByID(ctx context.Context, id int64) (*model.AssetBundle, error)
	FindByAssetID(ctx context.Context, assetID string) (*model.AssetBundle, error)
	List(ctx context.Context, f AssetBundleFilter) ([]*model.AssetBundle, int64, error)
	ListByAuthor(ctx context.Context, author string, limit int) ([]*model.AssetBundle, error)
	ListActive(ctx context.Context, limit int) ([]*model.AssetBundle, error)
	IncrementUseCount(ctx context.Context, assetID string) error
	ExistsByAssetID(ctx context.Context, assetID string) (bool, error)
}

// assetBundleRepo GORM 实现
type assetBundleRepo struct {
	db *gorm.DB
}

// NewAssetBundleRepository 构造资产包仓储
func NewAssetBundleRepository(db *gorm.DB) AssetBundleRepository {
	return &assetBundleRepo{db: db}
}

// Create 新建资产包
func (r *assetBundleRepo) Create(ctx context.Context, m *model.AssetBundle) error {
	if m == nil {
		return errors.New("asset bundle is nil")
	}
	return r.db.WithContext(ctx).Create(m).Error
}

// Update 更新资产包（全字段 save）
func (r *assetBundleRepo) Update(ctx context.Context, m *model.AssetBundle) error {
	if m == nil {
		return errors.New("asset bundle is nil")
	}
	return r.db.WithContext(ctx).Save(m).Error
}

// Save 等价 Update（语义别名）
func (r *assetBundleRepo) Save(ctx context.Context, m *model.AssetBundle) error {
	return r.Update(ctx, m)
}

// SoftDelete 软删除
func (r *assetBundleRepo) SoftDelete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.AssetBundle{}, id).Error
}

// HardDelete 硬删除
func (r *assetBundleRepo) HardDelete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Unscoped().Delete(&model.AssetBundle{}, id).Error
}

// FindByID 按主键查
func (r *assetBundleRepo) FindByID(ctx context.Context, id int64) (*model.AssetBundle, error) {
	var m model.AssetBundle
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// FindByAssetID 按业务键查
func (r *assetBundleRepo) FindByAssetID(ctx context.Context, assetID string) (*model.AssetBundle, error) {
	var m model.AssetBundle
	if err := r.db.WithContext(ctx).Where("asset_id = ?", assetID).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// List 多条件分页查询
func (r *assetBundleRepo) List(ctx context.Context, f AssetBundleFilter) ([]*model.AssetBundle, int64, error) {
	var list []*model.AssetBundle
	var total int64
	q := r.db.WithContext(ctx).Model(&model.AssetBundle{})

	if strings.TrimSpace(f.Keyword) != "" {
		kw := "%" + f.Keyword + "%"
		q = q.Where("title ILIKE ? OR description ILIKE ?", kw, kw)
	}
	if f.Author != "" {
		q = q.Where("author = ?", f.Author)
	}
	if f.Industry != "" {
		q = q.Where("industry = ?", f.Industry)
	}
	if f.Language != "" {
		q = q.Where("language = ?", f.Language)
	}
	if f.Scope != "" {
		q = q.Where("scope = ?", f.Scope)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if len(f.Tags) > 0 {
		// 用 && 数组包含查询（PostgreSQL text[] 包含语义）
		q = q.Where("tags && ?", f.Tags)
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.Size <= 0 {
		f.Size = 20
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("updated_at DESC").Offset((f.Page - 1) * f.Size).Limit(f.Size).Find(&list).Error
	return list, total, err
}

// ListByAuthor 列出某作者的所有资产包
func (r *assetBundleRepo) ListByAuthor(ctx context.Context, author string, limit int) ([]*model.AssetBundle, error) {
	var list []*model.AssetBundle
	q := r.db.WithContext(ctx).Where("author = ?", author).Order("updated_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&list).Error
	return list, err
}

// ListActive 列出启用的资产包
func (r *assetBundleRepo) ListActive(ctx context.Context, limit int) ([]*model.AssetBundle, error) {
	var list []*model.AssetBundle
	q := r.db.WithContext(ctx).Where("status = ?", model.AssetBundleStatusActive).Order("use_count DESC, updated_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&list).Error
	return list, err
}

// IncrementUseCount 累加使用次数
func (r *assetBundleRepo) IncrementUseCount(ctx context.Context, assetID string) error {
	return r.db.WithContext(ctx).Model(&model.AssetBundle{}).
		Where("asset_id = ?", assetID).
		UpdateColumn("use_count", gorm.Expr("use_count + 1")).Error
}

// ExistsByAssetID 检查业务键是否存在
func (r *assetBundleRepo) ExistsByAssetID(ctx context.Context, assetID string) (bool, error) {
	var c int64
	if err := r.db.WithContext(ctx).Model(&model.AssetBundle{}).
		Where("asset_id = ?", assetID).Count(&c).Error; err != nil {
		return false, err
	}
	return c > 0, nil
}
