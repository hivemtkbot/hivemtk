package service

import (
	"context"
	"fmt"
	"time"

	"marketing/internal/dto"
	"marketing/internal/model"

	"gorm.io/gorm"
)

// DouyinCardStatsService 抖音卡片统计服务接口
type DouyinCardStatsService interface {
	GetCardStats(ctx context.Context, req *dto.DouyinCardStatsRequest) (*dto.DouyinCardStatsResponse, error)
	GetOverallStats(ctx context.Context, req *dto.DouyinCardOverallStatsRequest) (*dto.DouyinCardOverallStatsResponse, error)
	RecordActivity(ctx context.Context, cardID uint, userID uint, action string, username, ipAddress, userAgent string) error
}

// douyinCardStatsService 抖音卡片统计服务实现
type douyinCardStatsService struct {
	db *gorm.DB
}

// NewDouyinCardStatsService 创建抖音卡片统计服务实例
func NewDouyinCardStatsService(db *gorm.DB) DouyinCardStatsService {
	return &douyinCardStatsService{
		db: db,
	}
}

// GetCardStats 获取单个卡片的统计数据
func (s *douyinCardStatsService) GetCardStats(ctx context.Context, req *dto.DouyinCardStatsRequest) (*dto.DouyinCardStatsResponse, error) {
	// 查询卡片信息
	var card model.DouyinCard
	if err := s.db.First(&card, req.CardID).Error; err != nil {
		return nil, fmt.Errorf("卡片不存在: %v", err)
	}

	// 构建基础查询条件
	baseQuery := s.db.Model(&model.DouyinCardActivity{}).Where("card_id = ?", req.CardID)

	// 如果指定了日期范围，添加日期过滤
	if req.StartDate != "" && req.EndDate != "" {
		baseQuery = baseQuery.Where("created_at >= ? AND created_at <= ?", req.StartDate, req.EndDate)
	}

	// 统计浏览量
	var views int64
	s.db.Model(&model.DouyinCardActivity{}).Where("card_id = ? AND action = ?", req.CardID, "view").Count(&views)

	// 按日期分组统计 - 仅支持浏览动作
	var dailyStats []dto.DailyStat
	query := baseQuery

	// 临时结构体用于查询结果
	type TempStat struct {
		Date   string `json:"date"`
		Action string `json:"action"`
		Count  int    `json:"count"`
	}

	var tempStats []TempStat

	if req.GroupBy == "day" {
		// 按天分组
		query.Select("DATE(created_at) as date, action, COUNT(*) as count").
			Group("DATE(created_at), action").
			Order("date, action").
			Scan(&tempStats)
	} else if req.GroupBy == "week" {
		// 按周分组
		query.Select("YEARWEEK(created_at) as date, action, COUNT(*) as count").
			Group("YEARWEEK(created_at), action").
			Order("date, action").
			Scan(&tempStats)
	} else if req.GroupBy == "month" {
		// 按月分组
		query.Select("DATE_FORMAT(created_at, '%Y-%m') as date, action, COUNT(*) as count").
			Group("DATE_FORMAT(created_at, '%Y-%m'), action").
			Order("date, action").
			Scan(&tempStats)
	}

	// 将临时统计结果转换为DailyStat
	// 先按日期分组
	dateMap := make(map[string]*dto.DailyStat)
	for _, stat := range tempStats {
		if _, exists := dateMap[stat.Date]; !exists {
			dateMap[stat.Date] = &dto.DailyStat{
				Date: stat.Date,
				View: 0,
			}
		}

		// 只处理浏览数据
		if stat.Action == "view" {
			dateMap[stat.Date].View = stat.Count
		}
	}

	// 转换为切片
	for _, stat := range dateMap {
		dailyStats = append(dailyStats, *stat)
	}

	// 构建响应
	response := &dto.DouyinCardStatsResponse{
		CardID:     req.CardID,
		Title:      card.Title,
		ViewCount:  int(views),
		DailyStats: dailyStats,
	}

	return response, nil
}

// GetOverallStats 获取所有卡片的总体统计数据
func (s *douyinCardStatsService) GetOverallStats(ctx context.Context, req *dto.DouyinCardOverallStatsRequest) (*dto.DouyinCardOverallStatsResponse, error) {
	// 获取卡片总数和激活数
	var totalCards, activeCards int64
	s.db.Model(&model.DouyinCard{}).Count(&totalCards)
	s.db.Model(&model.DouyinCard{}).Where("is_active = ?", true).Count(&activeCards)

	// 获取活动总数
	var totalActivities int64
	s.db.Model(&model.DouyinCardActivity{}).Count(&totalActivities)

	// 获取总浏览量
	var totalViews int64
	s.db.Model(&model.DouyinCardActivity{}).Where("action = ?", "view").Count(&totalViews)

	// 获取热门卡片 - 仅按浏览量排序
	var topCards []model.DouyinCard
	s.db.Order("view_count DESC").Limit(10).Find(&topCards)

	// 转换热门卡片数据
	var cards []dto.PopularCard
	for _, card := range topCards {
		cards = append(cards, dto.PopularCard{
			ID:        card.ID,
			Title:     card.Title,
			ViewCount: card.ViewCount,
			CreatedAt: card.CreatedAt.Format(time.RFC3339),
		})
	}

	// 获取按日期分组的统计数据
	var dailyStats []dto.DailyStat
	query := s.db.Model(&model.DouyinCardActivity{})

	if req.StartDate != "" {
		query = query.Where("created_at >= ?", req.StartDate)
	}

	if req.EndDate != "" {
		query = query.Where("created_at <= ?", req.EndDate)
	}

	// 临时结构体用于查询结果
	type TempStat struct {
		Date   string `json:"date"`
		Action string `json:"action"`
		Count  int    `json:"count"`
	}

	var tempStats []TempStat

	switch req.GroupBy {
	case "day":
		query = query.Select("DATE(created_at) as date, action, COUNT(*) as count").
			Group("DATE(created_at), action").
			Order("date, action")
	case "week":
		query = query.Select("YEARWEEK(created_at) as date, action, COUNT(*) as count").
			Group("YEARWEEK(created_at), action").
			Order("date, action")
	case "month":
		query = query.Select("DATE_FORMAT(created_at, '%Y-%m') as date, action, COUNT(*) as count").
			Group("DATE_FORMAT(created_at, '%Y-%m'), action").
			Order("date, action")
	default:
		query = query.Select("DATE(created_at) as date, action, COUNT(*) as count").
			Group("DATE(created_at), action").
			Order("date, action")
	}

	if err := query.Find(&tempStats).Error; err != nil {
		return nil, err
	}

	// 将临时统计结果转换为DailyStat
	// 先按日期分组
	dateMap := make(map[string]*dto.DailyStat)
	for _, stat := range tempStats {
		if _, exists := dateMap[stat.Date]; !exists {
			dateMap[stat.Date] = &dto.DailyStat{
				Date: stat.Date,
				View: 0,
			}
		}

		// 只处理浏览数据
		if stat.Action == "view" {
			dateMap[stat.Date].View = stat.Count
		}
	}

	// 转换为切片
	for _, stat := range dateMap {
		dailyStats = append(dailyStats, *stat)
	}

	// 获取最近的活动 - 仅浏览活动
	var recentActivities []model.DouyinCardActivity
	s.db.Where("action = ?", "view").Order("created_at DESC").Limit(10).Find(&recentActivities)

	// 转换活动数据
	var activities []dto.Activity
	for _, activity := range recentActivities {
		activities = append(activities, dto.Activity{
			ID:        activity.ID,
			CardID:    activity.CardID,
			Action:    activity.Action,
			Username:  activity.Username,
			IPAddress: activity.IPAddress,
			CreatedAt: activity.CreatedAt.Format(time.RFC3339),
		})
	}

	return &dto.DouyinCardOverallStatsResponse{
		TotalCards:     int(totalCards),
		ActiveCards:    int(activeCards),
		TotalViews:     int(totalViews),
		PopularCards:   cards,
		DailyStats:     dailyStats,
		RecentActivity: activities,
	}, nil
}

// RecordActivity 记录卡片活动
func (s *douyinCardStatsService) RecordActivity(ctx context.Context, cardID uint, userID uint, action string, username, ipAddress, userAgent string) error {
	// 只记录浏览活动
	if action != "view" {
		return nil
	}

	activity := &model.DouyinCardActivity{
		CardID:    cardID,
		UserID:    userID,
		Action:    action,
		Username:  username,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		CreatedAt: time.Now(),
	}

	if err := s.db.Create(activity).Error; err != nil {
		return fmt.Errorf("记录活动失败: %v", err)
	}

	// 更新卡片的浏览量统计
	s.db.Model(&model.DouyinCard{}).Where("id = ?", cardID).Update("view_count", gorm.Expr("view_count + 1"))

	return nil
}
