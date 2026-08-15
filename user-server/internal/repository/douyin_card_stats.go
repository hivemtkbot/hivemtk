package repository

import (
	"context"
	"fmt"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// DouyinCardStatsTempStat 按时间分组的临时统计结果（repository 层本地类型）
type DouyinCardStatsTempStat struct {
	Date   string
	Action string
	Count  int
}

// DouyinCardStatsRepository 抖音卡片统计仓库接口
type DouyinCardStatsRepository interface {
	GetCardByID(ctx context.Context, cardID uint) (*model.DouyinCard, error)
	CountCardViews(ctx context.Context, cardID uint) (int64, error)
	GetCardDailyStats(ctx context.Context, cardID uint, startDate, endDate, groupBy string) ([]DouyinCardStatsTempStat, error)
	GetRecentActivitiesByCard(ctx context.Context, cardID uint, limit int) ([]model.DouyinCardActivity, error)
	CountTotalCards(ctx context.Context) (int64, error)
	CountActiveCards(ctx context.Context) (int64, error)
	CountTotalViews(ctx context.Context) (int64, error)
	GetTopCards(ctx context.Context, limit int) ([]model.DouyinCard, error)
	GetOverallDailyStats(ctx context.Context, startDate, endDate, groupBy string) ([]DouyinCardStatsTempStat, error)
	GetRecentActivities(ctx context.Context, limit int) ([]model.DouyinCardActivity, error)
	CreateActivity(ctx context.Context, activity *model.DouyinCardActivity) error
	IncrementCardViewCount(ctx context.Context, cardID uint) error
}

// douyinCardStatsRepository 抖音卡片统计仓库实现
type douyinCardStatsRepository struct {
	db *gorm.DB
}

// NewDouyinCardStatsRepository 创建抖音卡片统计仓库
func NewDouyinCardStatsRepository(db *gorm.DB) DouyinCardStatsRepository {
	return &douyinCardStatsRepository{db: db}
}

// GetCardByID 按 ID 获取卡片
func (r *douyinCardStatsRepository) GetCardByID(ctx context.Context, cardID uint) (*model.DouyinCard, error) {
	var card model.DouyinCard
	if err := r.db.WithContext(ctx).First(&card, cardID).Error; err != nil {
		return nil, err
	}
	return &card, nil
}

// CountCardViews 统计卡片浏览量
func (r *douyinCardStatsRepository) CountCardViews(ctx context.Context, cardID uint) (int64, error) {
	var views int64
	err := r.db.WithContext(ctx).Model(&model.DouyinCardActivity{}).
		Where("card_id = ? AND action = ?", cardID, "view").
		Count(&views).Error
	return views, err
}

// applyDouyinStatsGroupBy 根据分组方式构建查询 Select/Group/Order
func applyDouyinStatsGroupBy(query *gorm.DB, groupBy string) *gorm.DB {
	switch groupBy {
	case "day":
		return query.Select("DATE(created_at) as date, action, COUNT(*) as count").
			Group("DATE(created_at), action").
			Order("date, action")
	case "week":
		return query.Select("YEARWEEK(created_at) as date, action, COUNT(*) as count").
			Group("YEARWEEK(created_at), action").
			Order("date, action")
	case "month":
		return query.Select("DATE_FORMAT(created_at, '%Y-%m') as date, action, COUNT(*) as count").
			Group("DATE_FORMAT(created_at, '%Y-%m'), action").
			Order("date, action")
	default:
		return query.Select("DATE(created_at) as date, action, COUNT(*) as count").
			Group("DATE(created_at), action").
			Order("date, action")
	}
}

// GetCardDailyStats 按时间分组获取卡片统计
func (r *douyinCardStatsRepository) GetCardDailyStats(ctx context.Context, cardID uint, startDate, endDate, groupBy string) ([]DouyinCardStatsTempStat, error) {
	query := r.db.WithContext(ctx).Model(&model.DouyinCardActivity{}).Where("card_id = ?", cardID)
	if startDate != "" && endDate != "" {
		query = query.Where("created_at >= ? AND created_at <= ?", startDate, endDate)
	}
	query = applyDouyinStatsGroupBy(query, groupBy)
	var stats []DouyinCardStatsTempStat
	if err := query.Scan(&stats).Error; err != nil {
		return nil, err
	}
	return stats, nil
}

// GetRecentActivitiesByCard 获取卡片的最近活动
func (r *douyinCardStatsRepository) GetRecentActivitiesByCard(ctx context.Context, cardID uint, limit int) ([]model.DouyinCardActivity, error) {
	var activities []model.DouyinCardActivity
	err := r.db.WithContext(ctx).Where("card_id = ? AND action = ?", cardID, "view").
		Order("created_at DESC").Limit(limit).Find(&activities).Error
	return activities, err
}

// CountTotalCards 统计卡片总数
func (r *douyinCardStatsRepository) CountTotalCards(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.DouyinCard{}).Count(&n).Error
	return n, err
}

// CountActiveCards 统计激活卡片数
func (r *douyinCardStatsRepository) CountActiveCards(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.DouyinCard{}).Where("is_active = ?", true).Count(&n).Error
	return n, err
}

// CountTotalViews 统计总浏览量
func (r *douyinCardStatsRepository) CountTotalViews(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.DouyinCardActivity{}).Where("action = ?", "view").Count(&n).Error
	return n, err
}

// GetTopCards 获取热门卡片
func (r *douyinCardStatsRepository) GetTopCards(ctx context.Context, limit int) ([]model.DouyinCard, error) {
	var cards []model.DouyinCard
	err := r.db.WithContext(ctx).Order("view_count DESC").Limit(limit).Find(&cards).Error
	return cards, err
}

// GetOverallDailyStats 获取全部卡片的按时间分组统计
func (r *douyinCardStatsRepository) GetOverallDailyStats(ctx context.Context, startDate, endDate, groupBy string) ([]DouyinCardStatsTempStat, error) {
	query := r.db.WithContext(ctx).Model(&model.DouyinCardActivity{})
	if startDate != "" {
		query = query.Where("created_at >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("created_at <= ?", endDate)
	}
	query = applyDouyinStatsGroupBy(query, groupBy)
	var stats []DouyinCardStatsTempStat
	if err := query.Find(&stats).Error; err != nil {
		return nil, err
	}
	return stats, nil
}

// GetRecentActivities 获取最近活动
func (r *douyinCardStatsRepository) GetRecentActivities(ctx context.Context, limit int) ([]model.DouyinCardActivity, error) {
	var activities []model.DouyinCardActivity
	err := r.db.WithContext(ctx).Where("action = ?", "view").
		Order("created_at DESC").Limit(limit).Find(&activities).Error
	return activities, err
}

// CreateActivity 创建活动记录
func (r *douyinCardStatsRepository) CreateActivity(ctx context.Context, activity *model.DouyinCardActivity) error {
	if err := r.db.WithContext(ctx).Create(activity).Error; err != nil {
		return fmt.Errorf("记录活动失败: %w", err)
	}
	return nil
}

// IncrementCardViewCount 增加卡片浏览量
func (r *douyinCardStatsRepository) IncrementCardViewCount(ctx context.Context, cardID uint) error {
	return r.db.WithContext(ctx).Model(&model.DouyinCard{}).
		Where("id = ?", cardID).
		Update("view_count", gorm.Expr("view_count + 1")).Error
}

