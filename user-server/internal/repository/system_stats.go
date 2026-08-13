// Package repository 提供系统监控统计的数据库访问层。
//
// 本文件封装 system_monitor service 中所需的全部统计查询。
// 严格遵循五层架构:Service 层不直接调用 db.GetDB(),必须通过 Repository。
//
// 注意:
//   - ctx 参数暂未在 GORM 链中显式透传,由 ctx 透传专项统一补全
//   - 不在 Repository 层做业务判断(仅按表聚合)
package repository

import (
	"context"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// SystemStatsRepository 系统统计仓库接口
type SystemStatsRepository interface {
	CountSystemUsers(ctx context.Context) (int64, error)
	CountActiveSystemUsers(ctx context.Context, sinceUnix int64) (int64, error)
	CountOrders(ctx context.Context) (int64, error)
	CountCards(ctx context.Context) (int64, error)
	CountShortLinks(ctx context.Context) (int64, error)
	CountTodayVisits(ctx context.Context, sinceUnix int64) (int64, error)
	CountEmailLists(ctx context.Context) (int64, error)
	CountEmailJobs(ctx context.Context) (int64, error)
	CountMaterials(ctx context.Context) (int64, error)
	ListRecentSystemMetrics(ctx context.Context, limit int) ([]model.SystemMetrics, error)
}

type systemStatsRepo struct {
	db *gorm.DB
}

// NewSystemStatsRepository 创建系统统计仓库实例
func NewSystemStatsRepository() SystemStatsRepository {
	return &systemStatsRepo{db: _db.GetDB()}
}

func (r *systemStatsRepo) CountSystemUsers(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.Model(&model.SystemUser{}).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *systemStatsRepo) CountActiveSystemUsers(ctx context.Context, sinceUnix int64) (int64, error) {
	var n int64
	// PostgreSQL 修复：sinceUnix 为 Unix epoch 秒（int64），
	// timestamp 字段与 int 直接比较时 PG 不会自动按 epoch 解释，
	// 必须显式 to_timestamp(?) 转换为 timestamp 后再比较。
	if err := r.db.Model(&model.SystemUser{}).Where("updated_at >= to_timestamp(?)", sinceUnix).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *systemStatsRepo) CountOrders(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.Model(&model.Order{}).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *systemStatsRepo) CountCards(ctx context.Context) (int64, error) {
	var total int64
	cardTables := []string{"douyin_cards", "kuaishou_cards", "xiaohongshu_cards", "xianyu_cards"}
	for _, table := range cardTables {
		var c int64
		if err := r.db.Table(table).Count(&c).Error; err != nil {
			continue
		}
		total += c
	}
	return total, nil
}

func (r *systemStatsRepo) CountShortLinks(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.Model(&model.ShortLink{}).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *systemStatsRepo) CountTodayVisits(ctx context.Context, sinceUnix int64) (int64, error) {
	var n int64
	// PostgreSQL 修复：sinceUnix 为 Unix epoch 秒（int64），
	// timestamp 字段与 int 直接比较时 PG 不会自动按 epoch 解释，
	// 必须显式 to_timestamp(?) 转换为 timestamp 后再比较。
	if err := r.db.Model(&model.VisitLog{}).Where("created_at >= to_timestamp(?)", sinceUnix).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *systemStatsRepo) CountEmailLists(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.Model(&model.EmailList{}).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *systemStatsRepo) CountEmailJobs(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.Model(&model.EmailJobs{}).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *systemStatsRepo) CountMaterials(ctx context.Context) (int64, error) {
	var n int64
	// 素材属 content 域私有实体，共享 repository 不跨域引用其 model，
	// 与 CountCards 同样按表名聚合（GORM 默认表名 materials）。
	if err := r.db.Table("materials").Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *systemStatsRepo) ListRecentSystemMetrics(ctx context.Context, limit int) ([]model.SystemMetrics, error) {
	var list []model.SystemMetrics
	if err := r.db.Limit(limit).Order("created_at DESC").Find(&list).Error; err != nil {
		return []model.SystemMetrics{}, err
	}
	return list, nil
}
