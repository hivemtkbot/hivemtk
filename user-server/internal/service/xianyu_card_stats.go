package service

import (
	"context"
	"fmt"
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/repository"
	"time"

	"gorm.io/gorm"
)

// XianyuCardStatsService 闲鱼卡片统计服务接口
type XianyuCardStatsService interface {
	GetCardStats(ctx context.Context, cardID uint, startDate, endDate string) (*dto.XianyuCardStatsResponse, error)
	GetOverallStats(ctx context.Context, startDate, endDate string) (*dto.XianyuCardOverallStatsResponse, error)
	// GetCardStatsRaw 返回原始统计数据（包含 views/clicks/shares 完整字段）
	// 供需要按日期明细展示 clicks/shares 的 controller 使用，避免丢失数据。
	GetCardStatsRaw(ctx context.Context, cardID uint, startDate, endDate string) (*dto.CardStatsData, error)
	// GetOverallStatsRaw 返回原始整体统计数据（包含完整的 StatsByDate 与 TopCards）
	GetOverallStatsRaw(ctx context.Context, startDate, endDate string) (*dto.OverallStatsData, error)
	RecordView(ctx context.Context, cardID uint, ip, userAgent, referer string) error
	RecordClick(ctx context.Context, cardID uint, ip, userAgent, referer string) error
	RecordShare(ctx context.Context, cardID uint, ip, userAgent, referer string) error
}

// xianyuCardStatsService 闲鱼卡片统计服务实现
type xianyuCardStatsService struct {
	repo repository.XianyuCardStatsRepository
}

// NewXianyuCardStatsService 创建闲鱼卡片统计服务
func NewXianyuCardStatsService(db any) XianyuCardStatsService {
	gormDB := db.(*gorm.DB)
	return &xianyuCardStatsService{
		repo: repository.NewXianyuCardStatsRepository(gormDB),
	}
}

// GetCardStats 获取卡片统计数据
func (s *xianyuCardStatsService) GetCardStats(ctx context.Context, cardID uint, startDate, endDate string) (*dto.XianyuCardStatsResponse, error) {
	// 解析日期
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, fmt.Errorf("开始日期格式错误: %w", err)
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, fmt.Errorf("结束日期格式错误: %w", err)
	}

	// 获取统计数据
	stats, err := s.repo.GetCardStats(ctx, cardID, start, end)
	if err != nil {
		return nil, fmt.Errorf("获取卡片统计数据失败: %w", err)
	}

	// 转换为响应格式
	// 转换 StatsByDate 到 DailyStat
	dailyStats := make([]dto.DailyStat, len(stats.StatsByDate))
	for i, stat := range stats.StatsByDate {
		dailyStats[i] = dto.DailyStat{
			Date: stat.Date,
			View: stat.Views,
		}
	}

	return &dto.XianyuCardStatsResponse{
		CardID:     cardID,
		Title:      "", // 需要从其他地方获取标题
		ViewCount:  stats.Views,
		ClickCount: stats.Clicks,
		ShareCount: stats.Shares,
		DailyStats: dailyStats,
	}, nil
}

// GetOverallStats 获取整体统计数据
func (s *xianyuCardStatsService) GetOverallStats(ctx context.Context, startDate, endDate string) (*dto.XianyuCardOverallStatsResponse, error) {
	// 解析日期
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, fmt.Errorf("开始日期格式错误: %w", err)
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, fmt.Errorf("结束日期格式错误: %w", err)
	}

	// 获取统计数据
	stats, err := s.repo.GetOverallStats(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("获取整体统计数据失败: %w", err)
	}

	// 转换为响应格式
	// 转换 StatsByDate 到 DailyStat
	dailyStats := make([]dto.DailyStat, len(stats.StatsByDate))
	for i, stat := range stats.StatsByDate {
		dailyStats[i] = dto.DailyStat{
			Date: stat.Date,
			View: stat.Views,
		}
	}

	// 转换 TopCards 到 PopularCard
	popularCards := make([]dto.PopularCard, len(stats.TopCards))
	for i, card := range stats.TopCards {
		popularCards[i] = dto.PopularCard{
			ID:        card.ID,
			Title:     card.Title,
			ViewCount: card.ViewCount,
			CreatedAt: card.CreatedAt,
		}
	}

	return &dto.XianyuCardOverallStatsResponse{
		TotalCards:     int(stats.TotalCards),
		ActiveCards:    int(stats.ActiveCards),
		TotalViews:     stats.TotalViewCount,
		TotalClicks:    stats.TotalClickCount,
		TotalShares:    stats.TotalShareCount,
		DailyStats:     dailyStats,
		PopularCards:   popularCards,
		RecentActivity: []dto.Activity{}, // 暂时为空，后续可以添加
	}, nil
}

// GetCardStatsRaw 返回原始卡片统计数据（包含 views/clicks/shares 完整字段）
func (s *xianyuCardStatsService) GetCardStatsRaw(ctx context.Context, cardID uint, startDate, endDate string) (*dto.CardStatsData, error) {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, fmt.Errorf("开始日期格式错误: %w", err)
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, fmt.Errorf("结束日期格式错误: %w", err)
	}
	result, err := s.repo.GetCardStats(ctx, cardID, start, end)
	if err != nil {
		return nil, err
	}
	// 转换 repository.CardStatsResult → dto.CardStatsData
	statsByDate := make([]dto.StatsByDate, len(result.StatsByDate))
	for i, sd := range result.StatsByDate {
		statsByDate[i] = dto.StatsByDate{
			Date:   sd.Date,
			Views:  sd.Views,
			Clicks: sd.Clicks,
			Shares: sd.Shares,
		}
	}
	return &dto.CardStatsData{
		CardID:      result.CardID,
		Views:       result.Views,
		Clicks:      result.Clicks,
		Shares:      result.Shares,
		StatsByDate: statsByDate,
	}, nil
}

// GetOverallStatsRaw 返回原始整体统计数据（包含完整的 StatsByDate 与 TopCards）
func (s *xianyuCardStatsService) GetOverallStatsRaw(ctx context.Context, startDate, endDate string) (*dto.OverallStatsData, error) {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, fmt.Errorf("开始日期格式错误: %w", err)
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, fmt.Errorf("结束日期格式错误: %w", err)
	}
	result, err := s.repo.GetOverallStats(ctx, start, end)
	if err != nil {
		return nil, err
	}
	// 转换 repository.CardOverallStatsResult → dto.OverallStatsData
	statsByDate := make([]dto.StatsByDate, len(result.StatsByDate))
	for i, sd := range result.StatsByDate {
		statsByDate[i] = dto.StatsByDate{
			Date:   sd.Date,
			Views:  sd.Views,
			Clicks: sd.Clicks,
			Shares: sd.Shares,
		}
	}
	topCards := make([]dto.TopCard, len(result.TopCards))
	for i, c := range result.TopCards {
		topCards[i] = dto.TopCard{
			ID:        c.ID,
			Title:     c.Title,
			ViewCount: c.ViewCount,
			CreatedAt: c.CreatedAt,
		}
	}
	return &dto.OverallStatsData{
		TotalViewCount:  result.TotalViewCount,
		TotalClickCount: result.TotalClickCount,
		TotalShareCount: result.TotalShareCount,
		TotalCards:      result.TotalCards,
		ActiveCards:     result.ActiveCards,
		StatsByDate:     statsByDate,
		TopCards:        topCards,
	}, nil
}

// RecordView 记录浏览
func (s *xianyuCardStatsService) RecordView(ctx context.Context, cardID uint, ip, userAgent, referer string) error {
	return s.repo.RecordActivity(ctx, cardID, "view", ip, userAgent, referer)
}

// RecordClick 记录点击
func (s *xianyuCardStatsService) RecordClick(ctx context.Context, cardID uint, ip, userAgent, referer string) error {
	return s.repo.RecordActivity(ctx, cardID, "click", ip, userAgent, referer)
}

// RecordShare 记录分享
func (s *xianyuCardStatsService) RecordShare(ctx context.Context, cardID uint, ip, userAgent, referer string) error {
	return s.repo.RecordActivity(ctx, cardID, "share", ip, userAgent, referer)
}
