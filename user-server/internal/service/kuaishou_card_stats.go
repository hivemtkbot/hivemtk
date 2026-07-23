package service

import (
	"fmt"
	"time"

	"marketing/internal/dto"
	"marketing/internal/model"

	"gorm.io/gorm"
	"context"
)

// KuaishouCardStatsService 快手卡片统计服务
type KuaishouCardStatsService struct {
	db *gorm.DB
}

// NewKuaishouCardStatsService 创建快手卡片统计服务实例
func NewKuaishouCardStatsService(db *gorm.DB) *KuaishouCardStatsService {
	return &KuaishouCardStatsService{db: db}
}

// GetCardStats 获取单个快手卡片的统计数据
func (s *KuaishouCardStatsService) GetCardStats(ctx context.Context, req *dto.KuaishouCardStatsRequest) (*dto.KuaishouCardStatsResponse, error) {
	// 获取卡片信息
	var card model.KuaishouCard
	if err := s.db.WithContext(ctx).First(&card, req.CardID).Error; err != nil {
		return nil, fmt.Errorf("卡片不存在: %w", err)
	}

	// 设置默认时间范围
	if req.StartDate.IsZero() {
		req.StartDate = time.Now().AddDate(0, -1, 0)	// 默认最近一个月
	}
	if req.EndDate.IsZero() {
		req.EndDate = time.Now()
	}

	// 设置默认分组方式
	if req.GroupBy == "" {
		req.GroupBy = "day"
	}

	// 获取活动统计数据
	var dailyStats []dto.DailyStat
	query := s.db.Table("kuaishou_card_activities").
		Select("DATE(created_at) as date, activity_type, COUNT(*) as count").
		Where("card_id = ? AND created_at BETWEEN ? AND ?", req.CardID, req.StartDate, req.EndDate).
		Group("DATE(created_at), activity_type")

	var results []struct {
		Date		string	`json:"date"`
		ActivityType	string	`json:"activity_type"`
		Count		int	`json:"count"`
	}

	if err := query.Scan(&results).Error; err != nil {
		return nil, fmt.Errorf("查询统计数据失败: %w", err)
	}

	// 将查询结果转换为DailyStat格式
	dateMap := make(map[string]*dto.DailyStat)
	for _, result := range results {
		dateStr := result.Date
		if _, exists := dateMap[dateStr]; !exists {
			dateMap[dateStr] = &dto.DailyStat{
				Date:	dateStr,
				View:	0,
			}
		}

		switch result.ActivityType {
		case "view":
			dateMap[dateStr].View = result.Count
		}
	}

	// 转换为切片并按日期排序
	for _, stat := range dateMap {
		dailyStats = append(dailyStats, *stat)
	}

	// 获取总统计数据
	var totalViews int64
	s.db.WithContext(ctx).Model(&model.KuaishouCardActivity{}).
		Where("card_id = ? AND activity_type = ?", req.CardID, "view").
		Count(&totalViews)

	// 构建响应
	response := &dto.KuaishouCardStatsResponse{
		CardID:		card.ID,
		CardTitle:	card.Title,
		TotalViews:	int(totalViews),
		DailyStats:	dailyStats,
	}

	return response, nil
}

// GetOverallStats 获取快手卡片总体统计数据
func (s *KuaishouCardStatsService) GetOverallStats(ctx context.Context, req *dto.KuaishouCardOverallStatsRequest) (*dto.KuaishouCardOverallStatsResponse, error) {
	// 设置默认时间范围
	if req.StartDate.IsZero() {
		req.StartDate = time.Now().AddDate(0, -1, 0)	// 默认最近一个月
	}
	if req.EndDate.IsZero() {
		req.EndDate = time.Now()
	}

	// 设置默认分组方式
	if req.GroupBy == "" {
		req.GroupBy = "day"
	}

	// 设置默认限制数量
	if req.Limit == 0 {
		req.Limit = 10
	}

	// 获取卡片总数和激活卡片数
	var totalCards, activeCards int64
	s.db.WithContext(ctx).Model(&model.KuaishouCard{}).Count(&totalCards)

	s.db.WithContext(ctx).Model(&model.KuaishouCard{}).
		Where("is_active = ?", true).
		Count(&activeCards)

	// 获取总统计数据
	var totalViews int64
	s.db.WithContext(ctx).Model(&model.KuaishouCardActivity{}).
		Where("activity_type = ? AND created_at BETWEEN ? AND ?", "view", req.StartDate, req.EndDate).
		Count(&totalViews)

	// 获取热门卡片
	var popularCards []model.KuaishouCard
	s.db.WithContext(ctx).Where("is_active = ?", true).
		Order("view_count DESC").
		Limit(req.Limit).
		Find(&popularCards)

	// 转换为DTO格式
	var popularCardDTOs []dto.PopularCard
	for _, card := range popularCards {
		popularCardDTOs = append(popularCardDTOs, dto.PopularCard{
			ID:		card.ID,
			Title:		card.Title,
			ViewCount:	card.ViewCount,
			CreatedAt:	card.CreatedAt.Format("2006-01-02"),
		})
	}

	// 获取每日统计数据
	var dailyStats []dto.DailyStat
	query := s.db.Table("kuaishou_card_activities").
		Select("DATE(created_at) as date, activity_type, COUNT(*) as count").
		Where("created_at BETWEEN ? AND ?", req.StartDate, req.EndDate).
		Group("DATE(created_at), activity_type")

	var results []struct {
		Date		string	`json:"date"`
		ActivityType	string	`json:"activity_type"`
		Count		int	`json:"count"`
	}

	if err := query.Scan(&results).Error; err != nil {
		return nil, fmt.Errorf("查询统计数据失败: %w", err)
	}

	// 将查询结果转换为DailyStat格式
	dateMap := make(map[string]*dto.DailyStat)
	for _, result := range results {
		dateStr := result.Date
		if _, exists := dateMap[dateStr]; !exists {
			dateMap[dateStr] = &dto.DailyStat{
				Date:	dateStr,
				View:	0,
			}
		}

		switch result.ActivityType {
		case "view":
			dateMap[dateStr].View = result.Count
		}
	}

	// 转换为切片并按日期排序
	for _, stat := range dateMap {
		dailyStats = append(dailyStats, *stat)
	}

	// 获取最近活动记录
	var recentActivities []dto.Activity
	s.db.Table("kuaishou_card_activities").
		Select("kuaishou_card_activities.id, kuaishou_card_activities.card_id, kuaishou_cards.title as card_title, kuaishou_card_activities.activity_type as action, kuaishou_card_activities.ip_address as user_ip, kuaishou_card_activities.user_agent, kuaishou_card_activities.extra_data, kuaishou_card_activities.created_at").
		Joins("LEFT JOIN kuaishou_cards ON kuaishou_card_activities.card_id = kuaishou_cards.id").
		Where("kuaishou_card_activities.activity_type = ?", "view").
		Order("kuaishou_card_activities.created_at DESC").
		Limit(10).
		Scan(&recentActivities)

	// 构建响应
	response := &dto.KuaishouCardOverallStatsResponse{
		TotalCards:		int(totalCards),
		ActiveCards:		int(activeCards),
		TotalViews:		int(totalViews),
		PopularCards:		popularCardDTOs,
		DailyStats:		dailyStats,
		RecentActivities:	recentActivities,
	}

	return response, nil
}

// RecordActivity 记录快手卡片活动
func (s *KuaishouCardStatsService) RecordActivity(ctx context.Context, cardID uint, action, userIP, userAgent, extraData string) error {
	// 只记录浏览活动
	if action != "view" {
		return nil
	}

	// 创建活动记录
	activity := model.KuaishouCardActivity{
		CardID:		cardID,
		ActivityType:	action,
		IPAddress:	userIP,
		UserAgent:	userAgent,
		ExtraData:	extraData,
	}

	if err := s.db.Create(&activity).Error; err != nil {
		return fmt.Errorf("记录活动失败: %w", err)
	}

	// 更新卡片统计
	var card model.KuaishouCard
	if err := s.db.WithContext(ctx).First(&card, cardID).Error; err != nil {
		return fmt.Errorf("获取卡片失败: %w", err)
	}

	switch action {
	case "view":
		card.ViewCount++
	}

	if err := s.db.Save(&card).Error; err != nil {
		return fmt.Errorf("更新卡片统计失败: %w", err)
	}

	return nil
}
