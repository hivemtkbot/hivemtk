package service

import (
	"fmt"
	"time"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/repository"

	"context"
	"gorm.io/gorm"
)

// KuaishouCardStatsService 快手卡片统计服务
type KuaishouCardStatsService struct {
	repo repository.KuaishouCardStatsRepository
}

// NewKuaishouCardStatsService 创建快手卡片统计服务实例
func NewKuaishouCardStatsService(db *gorm.DB) *KuaishouCardStatsService {
	return &KuaishouCardStatsService{repo: repository.NewKuaishouCardStatsRepository(db)}
}

// GetCardStats 获取单个快手卡片的统计数据
func (s *KuaishouCardStatsService) GetCardStats(ctx context.Context, req *dto.KuaishouCardStatsRequest) (*dto.KuaishouCardStatsResponse, error) {
	// 获取卡片信息
	card, err := s.repo.GetCardByID(ctx, req.CardID)
	if err != nil {
		return nil, fmt.Errorf("卡片不存在: %w", err)
	}

	// 设置默认时间范围
	if req.StartDate.IsZero() {
		req.StartDate = time.Now().AddDate(0, -1, 0) // 默认最近一个月
	}
	if req.EndDate.IsZero() {
		req.EndDate = time.Now()
	}

	// 设置默认分组方式
	if req.GroupBy == "" {
		req.GroupBy = "day"
	}

	// 获取活动统计数据
	results, err := s.repo.GetCardDailyStats(ctx, req.CardID, req.StartDate, req.EndDate)
	if err != nil {
		return nil, fmt.Errorf("查询统计数据失败: %w", err)
	}

	// 将查询结果转换为DailyStat格式
	var dailyStats []dto.DailyStat
	dateMap := make(map[string]*dto.DailyStat)
	for _, result := range results {
		dateStr := result.Date
		if _, exists := dateMap[dateStr]; !exists {
			dateMap[dateStr] = &dto.DailyStat{
				Date: dateStr,
				View: 0,
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
	totalViews, err := s.repo.CountCardViews(ctx, req.CardID)
	if err != nil {
		return nil, err
	}

	// 构建响应
	response := &dto.KuaishouCardStatsResponse{
		CardID:     card.ID,
		CardTitle:  card.Title,
		TotalViews: int(totalViews),
		DailyStats: dailyStats,
	}

	return response, nil
}

// GetOverallStats 获取快手卡片总体统计数据
func (s *KuaishouCardStatsService) GetOverallStats(ctx context.Context, req *dto.KuaishouCardOverallStatsRequest) (*dto.KuaishouCardOverallStatsResponse, error) {
	// 设置默认时间范围
	if req.StartDate.IsZero() {
		req.StartDate = time.Now().AddDate(0, -1, 0) // 默认最近一个月
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
	totalCards, err := s.repo.CountTotalCards(ctx)
	if err != nil {
		return nil, err
	}

	activeCards, err := s.repo.CountActiveCards(ctx)
	if err != nil {
		return nil, err
	}

	// 获取总统计数据
	totalViews, err := s.repo.CountTotalViews(ctx, req.StartDate, req.EndDate)
	if err != nil {
		return nil, err
	}

	// 获取热门卡片
	popularCards, err := s.repo.GetPopularCards(ctx, req.Limit)
	if err != nil {
		return nil, err
	}

	// 转换为DTO格式
	var popularCardDTOs []dto.PopularCard
	for _, card := range popularCards {
		popularCardDTOs = append(popularCardDTOs, dto.PopularCard{
			ID:        card.ID,
			Title:     card.Title,
			ViewCount: card.ViewCount,
			CreatedAt: card.CreatedAt.Format("2006-01-02"),
		})
	}

	// 获取每日统计数据
	results, err := s.repo.GetOverallDailyStats(ctx, req.StartDate, req.EndDate)
	if err != nil {
		return nil, fmt.Errorf("查询统计数据失败: %w", err)
	}

	// 将查询结果转换为DailyStat格式
	var dailyStats []dto.DailyStat
	dateMap := make(map[string]*dto.DailyStat)
	for _, result := range results {
		dateStr := result.Date
		if _, exists := dateMap[dateStr]; !exists {
			dateMap[dateStr] = &dto.DailyStat{
				Date: dateStr,
				View: 0,
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
	recentResults, err := s.repo.GetRecentActivitiesWithJoin(ctx, 10)
	if err != nil {
		return nil, err
	}
	var recentActivities []dto.Activity
	for _, r := range recentResults {
		recentActivities = append(recentActivities, dto.Activity{
			ID:        r.ID,
			CardID:    r.CardID,
			Action:    r.Action,
			IPAddress: r.UserIP,
			CreatedAt: r.CreatedAt.Format(time.RFC3339),
		})
	}

	// 构建响应
	response := &dto.KuaishouCardOverallStatsResponse{
		TotalCards:       int(totalCards),
		ActiveCards:      int(activeCards),
		TotalViews:       int(totalViews),
		PopularCards:     popularCardDTOs,
		DailyStats:       dailyStats,
		RecentActivities: recentActivities,
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
		CardID:       cardID,
		ActivityType: action,
		IPAddress:    userIP,
		UserAgent:    userAgent,
		ExtraData:    extraData,
	}

	if err := s.repo.CreateActivity(ctx, &activity); err != nil {
		return err
	}

	// 更新卡片统计
	card, err := s.repo.GetCardByID(ctx, cardID)
	if err != nil {
		return fmt.Errorf("获取卡片失败: %w", err)
	}

	switch action {
	case "view":
		card.ViewCount++
	}

	if err := s.repo.SaveCard(ctx, card); err != nil {
		return fmt.Errorf("更新卡片统计失败: %w", err)
	}

	return nil
}
