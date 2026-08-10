package repository

import (
	"context"
	"fmt"
	"time"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// KuaishouCardStatsDailyResult 按日期分组的统计结果（repository 层本地类型）
type KuaishouCardStatsDailyResult struct {
	Date         string
	ActivityType string
	Count        int
}

// KuaishouRecentActivity 快手最近活动记录（JOIN 查询结果）
type KuaishouRecentActivity struct {
	ID        uint
	CardID    uint
	CardTitle string
	Action    string
	UserIP    string
	UserAgent string
	ExtraData string
	CreatedAt time.Time
}

// KuaishouCardStatsRepository 快手卡片统计仓库接口
type KuaishouCardStatsRepository interface {
	GetCardByID(ctx context.Context, cardID uint) (*model.KuaishouCard, error)
	CountCardViews(ctx context.Context, cardID uint) (int64, error)
	GetCardDailyStats(ctx context.Context, cardID uint, startDate, endDate time.Time) ([]KuaishouCardStatsDailyResult, error)
	CountTotalCards(ctx context.Context) (int64, error)
	CountActiveCards(ctx context.Context) (int64, error)
	CountTotalViews(ctx context.Context, startDate, endDate time.Time) (int64, error)
	GetPopularCards(ctx context.Context, limit int) ([]model.KuaishouCard, error)
	GetOverallDailyStats(ctx context.Context, startDate, endDate time.Time) ([]KuaishouCardStatsDailyResult, error)
	GetRecentActivitiesWithJoin(ctx context.Context, limit int) ([]KuaishouRecentActivity, error)
	CreateActivity(ctx context.Context, activity *model.KuaishouCardActivity) error
	SaveCard(ctx context.Context, card *model.KuaishouCard) error
	// IncrementViewCount 原子自增浏览数，避免 service 层 read-modify-write 并发丢失更新
	IncrementViewCount(ctx context.Context, id uint) error
}

// kuaishouCardStatsRepository 快手卡片统计仓库实现
type kuaishouCardStatsRepository struct {
	db *gorm.DB
}

// NewKuaishouCardStatsRepository 创建快手卡片统计仓库
func NewKuaishouCardStatsRepository(db *gorm.DB) KuaishouCardStatsRepository {
	return &kuaishouCardStatsRepository{db: db}
}

// GetCardByID 按 ID 获取卡片
func (r *kuaishouCardStatsRepository) GetCardByID(ctx context.Context, cardID uint) (*model.KuaishouCard, error) {
	var card model.KuaishouCard
	if err := r.db.WithContext(ctx).First(&card, cardID).Error; err != nil {
		return nil, err
	}
	return &card, nil
}

// CountCardViews 统计卡片浏览量
func (r *kuaishouCardStatsRepository) CountCardViews(ctx context.Context, cardID uint) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.KuaishouCardActivity{}).
		Where("card_id = ? AND activity_type = ?", cardID, "view").
		Count(&n).Error
	return n, err
}

// GetCardDailyStats 按日期分组获取卡片统计
func (r *kuaishouCardStatsRepository) GetCardDailyStats(ctx context.Context, cardID uint, startDate, endDate time.Time) ([]KuaishouCardStatsDailyResult, error) {
	var results []KuaishouCardStatsDailyResult
	err := r.db.WithContext(ctx).Table("kuaishou_card_activities").
		Select("DATE(created_at) as date, activity_type, COUNT(*) as count").
		Where("card_id = ? AND created_at BETWEEN ? AND ?", cardID, startDate, endDate).
		Group("DATE(created_at), activity_type").
		Scan(&results).Error
	return results, err
}

// CountTotalCards 统计卡片总数
func (r *kuaishouCardStatsRepository) CountTotalCards(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.KuaishouCard{}).Count(&n).Error
	return n, err
}

// CountActiveCards 统计激活卡片数
func (r *kuaishouCardStatsRepository) CountActiveCards(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.KuaishouCard{}).Where("is_active = ?", true).Count(&n).Error
	return n, err
}

// CountTotalViews 统计时间范围内总浏览量
func (r *kuaishouCardStatsRepository) CountTotalViews(ctx context.Context, startDate, endDate time.Time) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.KuaishouCardActivity{}).
		Where("activity_type = ? AND created_at BETWEEN ? AND ?", "view", startDate, endDate).
		Count(&n).Error
	return n, err
}

// GetPopularCards 获取热门卡片
func (r *kuaishouCardStatsRepository) GetPopularCards(ctx context.Context, limit int) ([]model.KuaishouCard, error) {
	var cards []model.KuaishouCard
	err := r.db.WithContext(ctx).Where("is_active = ?", true).
		Order("view_count DESC").Limit(limit).Find(&cards).Error
	return cards, err
}

// GetOverallDailyStats 获取全部卡片的按日期分组统计
func (r *kuaishouCardStatsRepository) GetOverallDailyStats(ctx context.Context, startDate, endDate time.Time) ([]KuaishouCardStatsDailyResult, error) {
	var results []KuaishouCardStatsDailyResult
	err := r.db.WithContext(ctx).Table("kuaishou_card_activities").
		Select("DATE(created_at) as date, activity_type, COUNT(*) as count").
		Where("created_at BETWEEN ? AND ?", startDate, endDate).
		Group("DATE(created_at), activity_type").
		Scan(&results).Error
	return results, err
}

// GetRecentActivitiesWithJoin 获取最近活动（JOIN 卡片表获取标题）
func (r *kuaishouCardStatsRepository) GetRecentActivitiesWithJoin(ctx context.Context, limit int) ([]KuaishouRecentActivity, error) {
	var results []KuaishouRecentActivity
	err := r.db.WithContext(ctx).Table("kuaishou_card_activities").
		Select("kuaishou_card_activities.id, kuaishou_card_activities.card_id, kuaishou_cards.title as card_title, kuaishou_card_activities.activity_type as action, kuaishou_card_activities.ip_address as user_ip, kuaishou_card_activities.user_agent, kuaishou_card_activities.extra_data, kuaishou_card_activities.created_at").
		Joins("LEFT JOIN kuaishou_cards ON kuaishou_card_activities.card_id = kuaishou_cards.id").
		Where("kuaishou_card_activities.activity_type = ?", "view").
		Order("kuaishou_card_activities.created_at DESC").
		Limit(limit).
		Scan(&results).Error
	return results, err
}

// CreateActivity 创建活动记录
func (r *kuaishouCardStatsRepository) CreateActivity(ctx context.Context, activity *model.KuaishouCardActivity) error {
	if err := r.db.WithContext(ctx).Create(activity).Error; err != nil {
		return fmt.Errorf("记录活动失败: %w", err)
	}
	return nil
}

// SaveCard 保存卡片
func (r *kuaishouCardStatsRepository) SaveCard(ctx context.Context, card *model.KuaishouCard) error {
	return r.db.WithContext(ctx).Save(card).Error
}

// IncrementViewCount 原子自增浏览数（DB 层 view_count + 1），消除并发 read-modify-write 丢失更新
func (r *kuaishouCardStatsRepository) IncrementViewCount(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Model(&model.KuaishouCard{}).
		Where("id = ?", id).
		UpdateColumn("view_count", gorm.Expr("view_count + 1")).Error
}
