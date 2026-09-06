package repository

import (
	"context"
	"fmt"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// XiaohongshuCardStatsTempStat 按时间分组的临时统计结果（repository 层本地类型）
type XiaohongshuCardStatsTempStat struct {
	Date   string
	Action string
	Count  int
}

// XiaohongshuCardStatsRepository 小红书卡片统计仓库接口
type XiaohongshuCardStatsRepository interface {
	GetCardByID(ctx context.Context, cardID uint) (*model.XiaohongshuCard, error)
	CountCardViews(ctx context.Context, cardID uint) (int64, error)
	GetCardDailyStats(ctx context.Context, cardID uint, startDate, endDate, groupBy string) ([]XiaohongshuCardStatsTempStat, error)
	GetRecentActivitiesByCard(ctx context.Context, cardID uint, limit int) ([]model.XiaohongshuCardActivity, error)
	CountTotalCards(ctx context.Context) (int64, error)
	CountActiveCards(ctx context.Context) (int64, error)
	CountTotalViews(ctx context.Context) (int64, error)
	GetTopCards(ctx context.Context, limit int) ([]model.XiaohongshuCard, error)
	GetOverallDailyStats(ctx context.Context, startDate, endDate, groupBy string) ([]XiaohongshuCardStatsTempStat, error)
	GetRecentActivities(ctx context.Context, limit int) ([]model.XiaohongshuCardActivity, error)
	CreateActivity(ctx context.Context, activity *model.XiaohongshuCardActivity) error
	IncrementCardViewCount(ctx context.Context, cardID uint) error
}

type xiaohongshuCardStatsRepository struct {
	db *gorm.DB
}

// NewXiaohongshuCardStatsRepository 创建小红书卡片统计仓库
func NewXiaohongshuCardStatsRepository(db *gorm.DB) XiaohongshuCardStatsRepository {
	return &xiaohongshuCardStatsRepository{db: db}
}

func (r *xiaohongshuCardStatsRepository) GetCardByID(ctx context.Context, cardID uint) (*model.XiaohongshuCard, error) {
	var card model.XiaohongshuCard
	if err := r.db.WithContext(ctx).First(&card, cardID).Error; err != nil {
		return nil, err
	}
	return &card, nil
}

func (r *xiaohongshuCardStatsRepository) CountCardViews(ctx context.Context, cardID uint) (int64, error) {
	var views int64
	err := r.db.WithContext(ctx).Model(&model.XiaohongshuCardActivity{}).
		Where("card_id = ? AND activity_type = ?", cardID, "view").
		Count(&views).Error
	return views, err
}

func applyXiaohongshuStatsGroupBy(query *gorm.DB, groupBy string) *gorm.DB {
	switch groupBy {
	case "day":
		return query.Select("DATE(created_at) as date, activity_type as action, COUNT(*) as count").
			Group("DATE(created_at), activity_type").
			Order("date, action")
	case "week":
		return query.Select("YEARWEEK(created_at) as date, activity_type as action, COUNT(*) as count").
			Group("YEARWEEK(created_at), activity_type").
			Order("date, action")
	case "month":
		return query.Select("DATE_FORMAT(created_at, '%Y-%m') as date, activity_type as action, COUNT(*) as count").
			Group("DATE_FORMAT(created_at, '%Y-%m'), activity_type").
			Order("date, action")
	default:
		return query.Select("DATE(created_at) as date, activity_type as action, COUNT(*) as count").
			Group("DATE(created_at), activity_type").
			Order("date, action")
	}
}

func (r *xiaohongshuCardStatsRepository) GetCardDailyStats(ctx context.Context, cardID uint, startDate, endDate, groupBy string) ([]XiaohongshuCardStatsTempStat, error) {
	query := r.db.WithContext(ctx).Model(&model.XiaohongshuCardActivity{}).Where("card_id = ?", cardID)
	if startDate != "" && endDate != "" {
		query = query.Where("created_at >= ? AND created_at <= ?", startDate, endDate)
	}
	query = applyXiaohongshuStatsGroupBy(query, groupBy)
	var stats []XiaohongshuCardStatsTempStat
	if err := query.Scan(&stats).Error; err != nil {
		return nil, err
	}
	return stats, nil
}

func (r *xiaohongshuCardStatsRepository) GetRecentActivitiesByCard(ctx context.Context, cardID uint, limit int) ([]model.XiaohongshuCardActivity, error) {
	var activities []model.XiaohongshuCardActivity
	err := r.db.WithContext(ctx).Where("card_id = ? AND activity_type = ?", cardID, "view").
		Order("created_at DESC").Limit(limit).Find(&activities).Error
	return activities, err
}

func (r *xiaohongshuCardStatsRepository) CountTotalCards(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.XiaohongshuCard{}).Count(&n).Error
	return n, err
}

func (r *xiaohongshuCardStatsRepository) CountActiveCards(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.XiaohongshuCard{}).Where("is_active = ?", true).Count(&n).Error
	return n, err
}

func (r *xiaohongshuCardStatsRepository) CountTotalViews(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.XiaohongshuCardActivity{}).Where("activity_type = ?", "view").Count(&n).Error
	return n, err
}

func (r *xiaohongshuCardStatsRepository) GetTopCards(ctx context.Context, limit int) ([]model.XiaohongshuCard, error) {
	var cards []model.XiaohongshuCard
	err := r.db.WithContext(ctx).Order("view_count DESC").Limit(limit).Find(&cards).Error
	return cards, err
}

func (r *xiaohongshuCardStatsRepository) GetOverallDailyStats(ctx context.Context, startDate, endDate, groupBy string) ([]XiaohongshuCardStatsTempStat, error) {
	query := r.db.WithContext(ctx).Model(&model.XiaohongshuCardActivity{})
	if startDate != "" {
		query = query.Where("created_at >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("created_at <= ?", endDate)
	}
	query = applyXiaohongshuStatsGroupBy(query, groupBy)
	var stats []XiaohongshuCardStatsTempStat
	if err := query.Find(&stats).Error; err != nil {
		return nil, err
	}
	return stats, nil
}

func (r *xiaohongshuCardStatsRepository) GetRecentActivities(ctx context.Context, limit int) ([]model.XiaohongshuCardActivity, error) {
	var activities []model.XiaohongshuCardActivity
	err := r.db.WithContext(ctx).Where("activity_type = ?", "view").
		Order("created_at DESC").Limit(limit).Find(&activities).Error
	return activities, err
}

func (r *xiaohongshuCardStatsRepository) CreateActivity(ctx context.Context, activity *model.XiaohongshuCardActivity) error {
	if err := r.db.WithContext(ctx).Create(activity).Error; err != nil {
		return fmt.Errorf("记录活动失败: %w", err)
	}
	return nil
}

func (r *xiaohongshuCardStatsRepository) IncrementCardViewCount(ctx context.Context, cardID uint) error {
	return r.db.WithContext(ctx).Model(&model.XiaohongshuCard{}).
		Where("id = ?", cardID).
		Update("view_count", gorm.Expr("view_count + 1")).Error
}
