package service

import (
	"context"
	"fmt"
	"time"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/repository"

	"gorm.io/gorm"
)

// XiaohongshuCardStatsService 小红书卡片统计服务接口
type XiaohongshuCardStatsService interface {
	GetCardStats(ctx context.Context, req *dto.XiaohongshuCardStatsRequest) (*dto.XiaohongshuCardStatsResponse, error)
	GetOverallStats(ctx context.Context, req *dto.XiaohongshuCardOverallStatsRequest) (*dto.XiaohongshuCardOverallStatsResponse, error)
	RecordActivity(ctx context.Context, cardID uint, userID uint, action string, username, ipAddress, userAgent string) error
}

// xiaohongshuCardStatsService 小红书卡片统计服务实现
type xiaohongshuCardStatsService struct {
	repo repository.XiaohongshuCardStatsRepository
}

// NewXiaohongshuCardStatsService 创建小红书卡片统计服务实例
func NewXiaohongshuCardStatsService(db *gorm.DB) XiaohongshuCardStatsService {
	return &xiaohongshuCardStatsService{
		repo: repository.NewXiaohongshuCardStatsRepository(db),
	}
}

// GetCardStats 获取单个卡片的统计数据
func (s *xiaohongshuCardStatsService) GetCardStats(ctx context.Context, req *dto.XiaohongshuCardStatsRequest) (*dto.XiaohongshuCardStatsResponse, error) {
	// 查询卡片信息
	card, err := s.repo.GetCardByID(ctx, req.CardID)
	if err != nil {
		return nil, fmt.Errorf("卡片不存在: %v", err)
	}

	// 统计浏览量
	views, err := s.repo.CountCardViews(ctx, req.CardID)
	if err != nil {
		return nil, err
	}

	// 按日期分组统计 - 仅支持浏览动作
	tempStats, err := s.repo.GetCardDailyStats(ctx, req.CardID, req.StartDate, req.EndDate, req.GroupBy)
	if err != nil {
		return nil, err
	}
	dailyStats := convertXiaohongshuTempStats(tempStats)

	// 获取最近活动记录 - 仅浏览活动
	recentActivities, err := s.repo.GetRecentActivitiesByCard(ctx, req.CardID, 10)
	if err != nil {
		return nil, err
	}

	// 转换活动数据
	var activities []dto.Activity
	for _, activity := range recentActivities {
		activities = append(activities, dto.Activity{
			ID:        activity.ID,
			CardID:    activity.CardID,
			Action:    activity.ActivityType,
			Username:  activity.Username,
			IPAddress: activity.IPAddress,
			CreatedAt: activity.CreatedAt.Format(time.RFC3339),
		})
	}

	// 构建响应
	response := &dto.XiaohongshuCardStatsResponse{
		CardID:         req.CardID,
		Title:          card.Title,
		ViewCount:      int(views),
		DailyStats:     dailyStats,
		RecentActivity: activities,
	}

	return response, nil
}

// convertXiaohongshuTempStats 将仓库返回的临时统计结果转换为 DailyStat
func convertXiaohongshuTempStats(tempStats []repository.XiaohongshuCardStatsTempStat) []dto.DailyStat {
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
	var dailyStats []dto.DailyStat
	for _, stat := range dateMap {
		dailyStats = append(dailyStats, *stat)
	}
	return dailyStats
}

// GetOverallStats 获取所有卡片的总体统计数据
func (s *xiaohongshuCardStatsService) GetOverallStats(ctx context.Context, req *dto.XiaohongshuCardOverallStatsRequest) (*dto.XiaohongshuCardOverallStatsResponse, error) {
	// 获取卡片总数和激活数
	totalCards, err := s.repo.CountTotalCards(ctx)
	if err != nil {
		return nil, err
	}
	activeCards, err := s.repo.CountActiveCards(ctx)
	if err != nil {
		return nil, err
	}

	// 获取总浏览量
	totalViews, err := s.repo.CountTotalViews(ctx)
	if err != nil {
		return nil, err
	}

	// 获取热门卡片 - 仅按浏览量排序
	topCards, err := s.repo.GetTopCards(ctx, 10)
	if err != nil {
		return nil, err
	}

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
	tempStats, err := s.repo.GetOverallDailyStats(ctx, req.StartDate, req.EndDate, req.GroupBy)
	if err != nil {
		return nil, err
	}
	dailyStats := convertXiaohongshuTempStats(tempStats)

	// 获取最近的活动 - 仅浏览活动
	recentActivities, err := s.repo.GetRecentActivities(ctx, 10)
	if err != nil {
		return nil, err
	}

	// 转换活动数据
	var activities []dto.Activity
	for _, activity := range recentActivities {
		activities = append(activities, dto.Activity{
			ID:        activity.ID,
			CardID:    activity.CardID,
			Action:    activity.ActivityType,
			Username:  activity.Username,
			IPAddress: activity.IPAddress,
			CreatedAt: activity.CreatedAt.Format(time.RFC3339),
		})
	}

	return &dto.XiaohongshuCardOverallStatsResponse{
		TotalCards:     int(totalCards),
		ActiveCards:    int(activeCards),
		TotalViews:     int(totalViews),
		PopularCards:   cards,
		DailyStats:     dailyStats,
		RecentActivity: activities,
	}, nil
}

// RecordActivity 记录卡片活动
func (s *xiaohongshuCardStatsService) RecordActivity(ctx context.Context, cardID uint, userID uint, action string, username, ipAddress, userAgent string) error {
	// 只记录浏览活动
	if action != "view" {
		return nil
	}

	activity := &model.XiaohongshuCardActivity{
		CardID:       cardID,
		UserID:       userID,
		ActivityType: action,
		Username:     username,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		CreatedAt:    time.Now(),
	}

	if err := s.repo.CreateActivity(ctx, activity); err != nil {
		return err
	}

	// 更新卡片的浏览量统计
	return s.repo.IncrementCardViewCount(ctx, cardID)
}
