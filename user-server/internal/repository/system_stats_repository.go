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

	contentmodel "marketing/internal/content/model"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"

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
	CountAutoReplyAccounts(ctx context.Context) (int64, error)
	CountAutoReplyRules(ctx context.Context) (int64, error)
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
	if err := r.db.Model(&model.SystemUser{}).Where("updated_at >= ?", sinceUnix).Count(&n).Error; err != nil {
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
	if err := r.db.Model(&model.VisitLog{}).Where("created_at >= ?", sinceUnix).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *systemStatsRepo) CountAutoReplyAccounts(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.Model(&model.AutoReplyAccount{}).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *systemStatsRepo) CountAutoReplyRules(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.Model(&model.AutoReplyRule{}).Count(&n).Error; err != nil {
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
	if err := r.db.Model(&contentmodel.Material{}).Count(&n).Error; err != nil {
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
