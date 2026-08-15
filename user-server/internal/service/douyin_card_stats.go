package service

import (
	"context"
	"fmt"
	"time"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"

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
	repo repository.DouyinCardStatsRepository
}

// NewDouyinCardStatsService 创建抖音卡片统计服务实例
func NewDouyinCardStatsService(db *gorm.DB) DouyinCardStatsService {
	return &douyinCardStatsService{
		repo: repository.NewDouyinCardStatsRepository(db),
	}
}

// GetCardStats 获取单个卡片的统计数据
func (s *douyinCardStatsService) GetCardStats(ctx context.Context, req *dto.DouyinCardStatsRequest) (*dto.DouyinCardStatsResponse, error) {
	card, err := s.repo.GetCardByID(ctx, req.CardID)
	if err != nil {
		return nil, fmt.Errorf("卡片不存在: %v", err)
	}

	views, err := s.repo.CountCardViews(ctx, req.CardID)
	if err != nil {
		return nil, err
	}

	tempStats, err := s.repo.GetCardDailyStats(ctx, req.CardID, req.StartDate, req.EndDate, req.GroupBy)
	if err != nil {
		return nil, err
	}

	dailyStats := convertDouyinTempStats(tempStats)

	response := &dto.DouyinCardStatsResponse{
		CardID:     req.CardID,
		Title:      card.Title,
		ViewCount:  int(views),
		DailyStats: dailyStats,
	}

	return response, nil
}

// convertDouyinTempStats 将仓库返回的临时统计结果转换为 DailyStat
func convertDouyinTempStats(tempStats []repository.DouyinCardStatsTempStat) []dto.DailyStat {
	dateMap := make(map[string]*dto.DailyStat)
	for _, stat := range tempStats {
		if _, exists := dateMap[stat.Date]; !exists {
			dateMap[stat.Date] = &dto.DailyStat{
				Date: stat.Date,
				View: 0,
			}
		}

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
func (s *douyinCardStatsService) GetOverallStats(ctx context.Context, req *dto.DouyinCardOverallStatsRequest) (*dto.DouyinCardOverallStatsResponse, error) {
	totalCards, err := s.repo.CountTotalCards(ctx)
	if err != nil {
		return nil, err
	}
	activeCards, err := s.repo.CountActiveCards(ctx)
	if err != nil {
		return nil, err
	}

	totalViews, err := s.repo.CountTotalViews(ctx)
	if err != nil {
		return nil, err
	}

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

	tempStats, err := s.repo.GetOverallDailyStats(ctx, req.StartDate, req.EndDate, req.GroupBy)
	if err != nil {
		return nil, err
	}
	dailyStats := convertDouyinTempStats(tempStats)

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

	if err := s.repo.CreateActivity(ctx, activity); err != nil {
		return err
	}

	return s.repo.IncrementCardViewCount(ctx, cardID)
}

