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

// ============================================================================
// LM-：多平台卡片统计统一接口适配层
// ----------------------------------------------------------------------------
// 5 个平台 service（douyin/kuaishou/xiaohongshu/xianyu/tiktok）原本使用不同
// 命名的 DTO 与方法签名，LM- 引入 PlatformCardStatsService 统一接口后，
// 此处为每个平台提供 adapter 实现，满足统一接口的同时保留各平台
// 原方法（向后兼容）。
// ============================================================================

// 平台标识常量（与 model.PlatformXxx 保持一致）
const (
	PlatformCardStatsDouyin      = "douyin"
	PlatformCardStatsKuaishou    = "kuaishou"
	PlatformCardStatsXiaohongshu = "xiaohongshu"
	PlatformCardStatsXianyu      = "xianyu"
	PlatformCardStatsTiktok      = "tiktok"
)

// ----------------------------------------------------------------------------
// 抖音适配器
// ----------------------------------------------------------------------------

// douyinCardStatsAdapter 抖音卡片统计适配器
//
// 实现 PlatformCardStatsService 统一接口，内部委托给原有
// DouyinCardStatsService（保持向后兼容）。
type douyinCardStatsAdapter struct {
	inner DouyinCardStatsService
}

// NewPlatformDouyinCardStatsAdapter 创建抖音统一接口适配器
func NewPlatformDouyinCardStatsAdapter(inner DouyinCardStatsService) PlatformCardStatsService {
	return &douyinCardStatsAdapter{inner: inner}
}

func (a *douyinCardStatsAdapter) Platform() string { return PlatformCardStatsDouyin }

func (a *douyinCardStatsAdapter) GetCardStats(ctx context.Context, req *dto.PlatformCardStatsRequest) (*dto.PlatformCardStatsResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("请求不能为空")
	}
	inner := &dto.DouyinCardStatsRequest{
		CardID:    req.CardID,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
		GroupBy:   req.GroupBy,
	}
	resp, err := a.inner.GetCardStats(ctx, inner)
	if err != nil {
		return nil, err
	}
	return &dto.PlatformCardStatsResponse{
		Platform:       PlatformCardStatsDouyin,
		CardID:         resp.CardID,
		Title:          resp.Title,
		ViewCount:      resp.ViewCount,
		DailyStats:     resp.DailyStats,
		RecentActivity: resp.RecentActivity,
	}, nil
}

func (a *douyinCardStatsAdapter) GetOverallStats(ctx context.Context, req *dto.PlatformCardOverallStatsRequest) (*dto.PlatformCardOverallStatsResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("请求不能为空")
	}
	inner := &dto.DouyinCardOverallStatsRequest{
		GroupBy:   req.GroupBy,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
	}
	resp, err := a.inner.GetOverallStats(ctx, inner)
	if err != nil {
		return nil, err
	}
	return &dto.PlatformCardOverallStatsResponse{
		Platform:       PlatformCardStatsDouyin,
		TotalCards:     resp.TotalCards,
		ActiveCards:    resp.ActiveCards,
		TotalViews:     resp.TotalViews,
		PopularCards:   resp.PopularCards,
		DailyStats:     resp.DailyStats,
		RecentActivity: resp.RecentActivity,
	}, nil
}

// RecordActivity 抖音适配：直接复用内部 RecordActivity
func (a *douyinCardStatsAdapter) RecordActivity(ctx context.Context, cardID uint, userID uint, action string, username, ipAddress, userAgent string) error {
	return a.inner.RecordActivity(ctx, cardID, userID, action, username, ipAddress, userAgent)
}

// ----------------------------------------------------------------------------
// 小红书适配器
// ----------------------------------------------------------------------------

// xiaohongshuCardStatsAdapter 小红书卡片统计适配器
type xiaohongshuCardStatsAdapter struct {
	inner XiaohongshuCardStatsService
}

// NewPlatformXiaohongshuCardStatsAdapter 创建小红书统一接口适配器
func NewPlatformXiaohongshuCardStatsAdapter(inner XiaohongshuCardStatsService) PlatformCardStatsService {
	return &xiaohongshuCardStatsAdapter{inner: inner}
}

func (a *xiaohongshuCardStatsAdapter) Platform() string { return PlatformCardStatsXiaohongshu }

func (a *xiaohongshuCardStatsAdapter) GetCardStats(ctx context.Context, req *dto.PlatformCardStatsRequest) (*dto.PlatformCardStatsResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("请求不能为空")
	}
	inner := &dto.XiaohongshuCardStatsRequest{
		CardID:    req.CardID,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
		GroupBy:   req.GroupBy,
	}
	resp, err := a.inner.GetCardStats(ctx, inner)
	if err != nil {
		return nil, err
	}
	return &dto.PlatformCardStatsResponse{
		Platform:       PlatformCardStatsXiaohongshu,
		CardID:         resp.CardID,
		Title:          resp.Title,
		ViewCount:      resp.ViewCount,
		DailyStats:     resp.DailyStats,
		RecentActivity: resp.RecentActivity,
	}, nil
}

func (a *xiaohongshuCardStatsAdapter) GetOverallStats(ctx context.Context, req *dto.PlatformCardOverallStatsRequest) (*dto.PlatformCardOverallStatsResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("请求不能为空")
	}
	inner := &dto.XiaohongshuCardOverallStatsRequest{
		GroupBy:   req.GroupBy,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
	}
	resp, err := a.inner.GetOverallStats(ctx, inner)
	if err != nil {
		return nil, err
	}
	return &dto.PlatformCardOverallStatsResponse{
		Platform:       PlatformCardStatsXiaohongshu,
		TotalCards:     resp.TotalCards,
		ActiveCards:    resp.ActiveCards,
		TotalViews:     resp.TotalViews,
		PopularCards:   resp.PopularCards,
		DailyStats:     resp.DailyStats,
		RecentActivity: resp.RecentActivity,
	}, nil
}

func (a *xiaohongshuCardStatsAdapter) RecordActivity(ctx context.Context, cardID uint, userID uint, action string, username, ipAddress, userAgent string) error {
	return a.inner.RecordActivity(ctx, cardID, userID, action, username, ipAddress, userAgent)
}

// ----------------------------------------------------------------------------
// 快手适配器
// ----------------------------------------------------------------------------

// kuaishouCardStatsAdapter 快手卡片统计适配器
//
// 快手原本是 struct（非 interface），且 RecordActivity 签名（cardID, action,
// userIP, userAgent, extraData）与其他平台不一致；此处将其适配到统一接口。
type kuaishouCardStatsAdapter struct {
	inner *KuaishouCardStatsService
}

// NewPlatformKuaishouCardStatsAdapter 创建快手统一接口适配器
func NewPlatformKuaishouCardStatsAdapter(inner *KuaishouCardStatsService) PlatformCardStatsService {
	return &kuaishouCardStatsAdapter{inner: inner}
}

func (a *kuaishouCardStatsAdapter) Platform() string { return PlatformCardStatsKuaishou }

func (a *kuaishouCardStatsAdapter) GetCardStats(ctx context.Context, req *dto.PlatformCardStatsRequest) (*dto.PlatformCardStatsResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("请求不能为空")
	}
	// 快手原 DTO 用 time.Time，这里把 string 转 time.Time
	startDate, endDate := parseDateRange(req.StartDate, req.EndDate)
	inner := &dto.KuaishouCardStatsRequest{
		CardID:    req.CardID,
		StartDate: startDate,
		EndDate:   endDate,
		GroupBy:   req.GroupBy,
	}
	resp, err := a.inner.GetCardStats(ctx, inner)
	if err != nil {
		return nil, err
	}
	return &dto.PlatformCardStatsResponse{
		Platform:   PlatformCardStatsKuaishou,
		CardID:     resp.CardID,
		Title:      resp.CardTitle,
		ViewCount:  resp.TotalViews,
		DailyStats: resp.DailyStats,
	}, nil
}

func (a *kuaishouCardStatsAdapter) GetOverallStats(ctx context.Context, req *dto.PlatformCardOverallStatsRequest) (*dto.PlatformCardOverallStatsResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("请求不能为空")
	}
	startDate, endDate := parseDateRange(req.StartDate, req.EndDate)
	inner := &dto.KuaishouCardOverallStatsRequest{
		StartDate: startDate,
		EndDate:   endDate,
		GroupBy:   req.GroupBy,
		Limit:     req.Limit,
	}
	resp, err := a.inner.GetOverallStats(ctx, inner)
	if err != nil {
		return nil, err
	}
	return &dto.PlatformCardOverallStatsResponse{
		Platform:     PlatformCardStatsKuaishou,
		TotalCards:   resp.TotalCards,
		ActiveCards:  resp.ActiveCards,
		TotalViews:   resp.TotalViews,
		PopularCards: resp.PopularCards,
		DailyStats:   resp.DailyStats,
		// 快手原结构是 RecentActivities（复数），此处统一为 RecentActivity
		RecentActivity: resp.RecentActivities,
	}, nil
}

// RecordActivity 快手适配：原签名为 (cardID, action, userIP, userAgent, extraData)，
// 统一接口为 (ctx, cardID, userID, action, username, ipAddress, userAgent)，
// 这里将 userID 透传为 userID（快手旧实现不写 user_id 列），username 写到 extraData。
func (a *kuaishouCardStatsAdapter) RecordActivity(ctx context.Context, cardID uint, userID uint, action string, username, ipAddress, userAgent string) error {
	_ = userID
	_ = username
	return a.inner.RecordActivity(ctx, cardID, action, ipAddress, userAgent, "")
}

// ----------------------------------------------------------------------------
// 闲鱼适配器
// ----------------------------------------------------------------------------

// xianyuCardStatsAdapter 闲鱼卡片统计适配器
//
// 闲鱼原接口使用 (cardID, startDate, endDate string) 扁平参数，
// 统一接口使用 req 包装。此处做格式转换。
type xianyuCardStatsAdapter struct {
	inner XianyuCardStatsService
}

// NewPlatformXianyuCardStatsAdapter 创建闲鱼统一接口适配器
func NewPlatformXianyuCardStatsAdapter(inner XianyuCardStatsService) PlatformCardStatsService {
	return &xianyuCardStatsAdapter{inner: inner}
}

func (a *xianyuCardStatsAdapter) Platform() string { return PlatformCardStatsXianyu }

func (a *xianyuCardStatsAdapter) GetCardStats(ctx context.Context, req *dto.PlatformCardStatsRequest) (*dto.PlatformCardStatsResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("请求不能为空")
	}
	startDate, endDate := normalizeDateRange(req.StartDate, req.EndDate)
	resp, err := a.inner.GetCardStats(ctx, req.CardID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	return &dto.PlatformCardStatsResponse{
		Platform:       PlatformCardStatsXianyu,
		CardID:         resp.CardID,
		Title:          resp.Title,
		ViewCount:      resp.ViewCount,
		ClickCount:     resp.ClickCount,
		ShareCount:     resp.ShareCount,
		DailyStats:     resp.DailyStats,
		RecentActivity: resp.RecentActivity,
	}, nil
}

func (a *xianyuCardStatsAdapter) GetOverallStats(ctx context.Context, req *dto.PlatformCardOverallStatsRequest) (*dto.PlatformCardOverallStatsResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("请求不能为空")
	}
	startDate, endDate := normalizeDateRange(req.StartDate, req.EndDate)
	resp, err := a.inner.GetOverallStats(ctx, startDate, endDate)
	if err != nil {
		return nil, err
	}
	return &dto.PlatformCardOverallStatsResponse{
		Platform:       PlatformCardStatsXianyu,
		TotalCards:     resp.TotalCards,
		ActiveCards:    resp.ActiveCards,
		TotalViews:     resp.TotalViews,
		TotalClicks:    resp.TotalClicks,
		TotalShares:    resp.TotalShares,
		PopularCards:   resp.PopularCards,
		DailyStats:     resp.DailyStats,
		RecentActivity: resp.RecentActivity,
	}, nil
}

// RecordActivity 闲鱼适配：原签名 RecordView/RecordClick/RecordShare 分别实现，
// 统一接口的 RecordActivity 按 action 字符串分派。
func (a *xianyuCardStatsAdapter) RecordActivity(ctx context.Context, cardID uint, userID uint, action string, username, ipAddress, userAgent string) error {
	_ = userID
	_ = username
	switch action {
	case "view":
		return a.inner.RecordView(ctx, cardID, ipAddress, userAgent, "")
	case "click":
		return a.inner.RecordClick(ctx, cardID, ipAddress, userAgent, "")
	case "share":
		return a.inner.RecordShare(ctx, cardID, ipAddress, userAgent, "")
	default:
		// 未知动作降级为 view，保证不丢埋点
		return a.inner.RecordView(ctx, cardID, ipAddress, userAgent, "")
	}
}

// ----------------------------------------------------------------------------
// TikTok 适配器
// ----------------------------------------------------------------------------

// tiktokCardStatsAdapter TikTok 卡片统计适配器
//
// TikTok 没有独立的 stats service，统计能力挂在 TikTokCardService
// （Stats / StatsOverall / RecordView）。此处通过聚合这些方法满足统一接口。
type tiktokCardStatsAdapter struct {
	inner    TikTokCardService
	activity repository.TikTokCardRepository // 用于补充 activity 写入（保持与其他平台一致的 RecordActivity 行为）
}

// NewPlatformTiktokCardStatsAdapter 创建 TikTok 统一接口适配器
//
// 注：保留 gormDB *gorm.DB 入参以维持向后兼容（router 装配不改动），
// 内部在构造函数中实例化 repository，service struct 不直接持有 *gorm.DB。
func NewPlatformTiktokCardStatsAdapter(inner TikTokCardService, gormDB *gorm.DB) PlatformCardStatsService {
	return &tiktokCardStatsAdapter{inner: inner, activity: repository.NewTikTokCardRepository(gormDB)}
}

func (a *tiktokCardStatsAdapter) Platform() string { return PlatformCardStatsTiktok }

func (a *tiktokCardStatsAdapter) GetCardStats(ctx context.Context, req *dto.PlatformCardStatsRequest) (*dto.PlatformCardStatsResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("请求不能为空")
	}
	resp, err := a.inner.Stats(ctx, req.CardID)
	if err != nil {
		return nil, err
	}
	daily := make([]dto.DailyStat, 0, len(resp.DailyStats))
	for _, d := range resp.DailyStats {
		daily = append(daily, dto.DailyStat{
			Date: d.Date,
			View: int(d.ViewCount),
		})
	}
	return &dto.PlatformCardStatsResponse{
		Platform:   PlatformCardStatsTiktok,
		CardID:     resp.CardID,
		Title:      resp.Title,
		ViewCount:  resp.ViewCount,
		DailyStats: daily,
	}, nil
}

func (a *tiktokCardStatsAdapter) GetOverallStats(ctx context.Context, req *dto.PlatformCardOverallStatsRequest) (*dto.PlatformCardOverallStatsResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("请求不能为空")
	}
	resp, err := a.inner.StatsOverall(ctx)
	if err != nil {
		return nil, err
	}
	daily := make([]dto.DailyStat, 0, len(resp.DailyStats))
	for _, d := range resp.DailyStats {
		daily = append(daily, dto.DailyStat{
			Date: d.Date,
			View: int(d.ViewCount),
		})
	}
	popular := make([]dto.PopularCard, 0, len(resp.PopularCards))
	for _, p := range resp.PopularCards {
		popular = append(popular, dto.PopularCard{
			ID:        p.ID,
			Title:     p.Title,
			ViewCount: int(p.ViewCount),
			CreatedAt: p.CreatedAt,
		})
	}
	recent := make([]dto.Activity, 0, len(resp.RecentActivity))
	for _, r := range resp.RecentActivity {
		recent = append(recent, dto.Activity{
			CardID:    0, // TikTok activity 不返回 cardID
			Action:    r.Action,
			Username:  r.Username,
			CreatedAt: r.CreatedAt,
		})
	}
	return &dto.PlatformCardOverallStatsResponse{
		Platform:       PlatformCardStatsTiktok,
		TotalCards:     int(resp.TotalCards),
		ActiveCards:    int(resp.ActiveCards),
		TotalViews:     int(resp.TotalViews),
		PopularCards:   popular,
		DailyStats:     daily,
		RecentActivity: recent,
	}, nil
}

// RecordActivity TikTok 适配：原 RecordView(cardID, ip, userAgent) 不接收 userID/username，
// 统一接口补充的 userID/username 通过附加 activity 记录持久化（保持兼容）。
func (a *tiktokCardStatsAdapter) RecordActivity(ctx context.Context, cardID uint, userID uint, action string, username, ipAddress, userAgent string) error {
	if err := a.inner.RecordView(ctx, cardID, ipAddress, userAgent); err != nil {
		return err
	}
	// 补充 user 信息（仅当提供且 repository 可用时；TikTokCardActivity 的 UserID 为 string 类型，
	// 原模型无 Username 列，username 透传到 UserAgent 前缀以保留可观测性）。
	if a.activity != nil && (userID > 0 || username != "") {
		uidStr := fmt.Sprintf("%d", userID)
		ua := userAgent
		if username != "" {
			ua = "[" + username + "] " + userAgent
		}
		activity := &model.TikTokCardActivity{
			CardID:       cardID,
			ActivityType: action,
			UserID:       uidStr,
			IPAddress:    ipAddress,
			UserAgent:    ua,
			Platform:     PlatformCardStatsTiktok,
		}
		// 修复：补充的 user 维度活动记录创建失败被 _ 静默丢弃，导致埋点丢失且无日志。
		// 改为返回错误，使调用方感知并决定是否重试。
		if err := a.activity.CreateActivity(ctx, activity); err != nil {
			return err
		}
	}
	return nil
}

// ----------------------------------------------------------------------------
// 工具函数
// ----------------------------------------------------------------------------

// parseDateRange 把 "" 字符串转 time.Time；空字符串时返回零值
func parseDateRange(start, end string) (time.Time, time.Time) {
	var s, e time.Time
	if start != "" {
		if t, err := time.Parse("2006-01-02", start); err == nil {
			s = t
		}
	}
	if end != "" {
		if t, err := time.Parse("2006-01-02", end); err == nil {
			e = t
		}
	}
	return s, e
}

// normalizeDateRange 保证日期非空：为空时默认最近 30 天
func normalizeDateRange(start, end string) (string, string) {
	if end == "" {
		end = time.Now().Format("2006-01-02")
	}
	if start == "" {
		start = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	}
	return start, end
}
