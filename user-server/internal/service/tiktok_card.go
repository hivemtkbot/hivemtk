package service

import (
	"context"
	"fmt"
	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/repository"
	"time"

	"gorm.io/gorm"
)

// TikTokCardService TikTok 卡片服务接口
type TikTokCardService interface {
	Create(ctx context.Context, req *dto.TikTokCardCreateRequest) (*dto.TikTokCardResponse, error)
	Update(ctx context.Context, req *dto.TikTokCardUpdateRequest) (*dto.TikTokCardResponse, error)
	Delete(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*dto.TikTokCardResponse, error)
	GetCardModelByID(ctx context.Context, id uint) (*model.TikTokCard, error)
	GetList(ctx context.Context, req *dto.TikTokCardListRequest) (*dto.TikTokCardListResponse, error)
	GenerateShortLink(ctx context.Context, cardID uint) (*dto.TikTokCardResponse, error)
	StatsOverall(ctx context.Context) (*dto.TikTokCardStatsOverallResponse, error)
	Stats(ctx context.Context, cardID uint) (*dto.TikTokCardStatsDetailResponse, error)
	RecordView(ctx context.Context, cardID uint, ip, userAgent string) error
}

// tiktokCardService TikTok 卡片服务实现
type tiktokCardService struct {
	repo repository.TikTokCardRepository
	db   *gorm.DB
}

// NewTikTokCardService 创建 TikTok 卡片服务
func NewTikTokCardService() TikTokCardService {
	return &tiktokCardService{
		repo: repository.NewTikTokCardRepository(db.GetDB()),
		db:   db.GetDB(),
	}
}

// NewTikTokCardServiceWithDB 通过 DB 创建 TikTok 卡片服务(用于测试)
func NewTikTokCardServiceWithDB(gdb *gorm.DB) TikTokCardService {
	return &tiktokCardService{
		repo: repository.NewTikTokCardRepository(gdb),
		db:   gdb,
	}
}

// Create 创建 TikTok 卡片
func (s *tiktokCardService) Create(ctx context.Context, req *dto.TikTokCardCreateRequest) (*dto.TikTokCardResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("请求不能为空")
	}
	if req.Title == "" {
		return nil, fmt.Errorf("标题不能为空")
	}

	redirectURL := req.RedirectURL
	if redirectURL == "" {
		redirectURL = "https://www.tiktok.com"
	}

	card := &model.TikTokCard{
		Title:        req.Title,
		Description:  req.Description,
		ImageURL:     req.ImageURL,
		RedirectURL:  redirectURL,
		DomainPoolID: req.DomainPoolID,
		Tags:         req.Tags,
		IsActive:     req.IsActive,
	}

	created, err := s.repo.Create(card)
	if err != nil {
		return nil, fmt.Errorf("创建 TikTok 卡片失败: %w", err)
	}

	return s.toResponse(created, ""), nil
}

// Update 更新 TikTok 卡片
func (s *tiktokCardService) Update(ctx context.Context, req *dto.TikTokCardUpdateRequest) (*dto.TikTokCardResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("请求不能为空")
	}

	card, err := s.repo.GetByID(req.ID)
	if err != nil {
		return nil, fmt.Errorf("卡片不存在: %w", err)
	}

	redirectURL := req.RedirectURL
	if redirectURL == "" {
		redirectURL = card.RedirectURL
		if redirectURL == "" {
			redirectURL = "https://www.tiktok.com"
		}
	}

	card.Title = req.Title
	card.Description = req.Description
	card.ImageURL = req.ImageURL
	card.RedirectURL = redirectURL
	card.DomainPoolID = req.DomainPoolID
	card.Tags = req.Tags
	card.ViewCount = req.ViewCount
	card.LikeCount = req.LikeCount
	card.ShareCount = req.ShareCount
	card.IsActive = req.IsActive

	updated, err := s.repo.Update(card)
	if err != nil {
		return nil, fmt.Errorf("更新 TikTok 卡片失败: %w", err)
	}
	return s.toResponse(updated, ""), nil
}

// Delete 删除 TikTok 卡片
func (s *tiktokCardService) Delete(ctx context.Context, id uint) error {
	// 先检查卡片是否存在
	_, err := s.repo.GetByID(id)
	if err != nil {
		return fmt.Errorf("卡片不存在: %w", err)
	}
	return s.repo.Delete(id)
}

// GetByID 获取 TikTok 卡片
func (s *tiktokCardService) GetByID(ctx context.Context, id uint) (*dto.TikTokCardResponse, error) {
	card, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	// 获取短链信息
	shortCode := ""
	if card.ShortLinkID != 0 {
		var sl model.ShortLink
		if err := s.db.First(&sl, card.ShortLinkID).Error; err == nil {
			shortCode = sl.ShortCode
		}
	}
	return s.toResponse(card, shortCode), nil
}

// GetCardModelByID 获取卡片模型(用于内部操作)
func (s *tiktokCardService) GetCardModelByID(ctx context.Context, id uint) (*model.TikTokCard, error) {
	return s.repo.GetByID(id)
}

// GetList 获取 TikTok 卡片列表
func (s *tiktokCardService) GetList(ctx context.Context, req *dto.TikTokCardListRequest) (*dto.TikTokCardListResponse, error) {
	if req == nil {
		req = &dto.TikTokCardListRequest{Page: 1, PageSize: 20}
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}

	cards, total, err := s.repo.GetList(req)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.TikTokCardResponse, 0, len(cards))
	for i := range cards {
		responses = append(responses, *s.toResponse(&cards[i], ""))
	}
	return &dto.TikTokCardListResponse{List: responses, Total: total}, nil
}

// GenerateShortLink 为卡片生成短链
func (s *tiktokCardService) GenerateShortLink(ctx context.Context, cardID uint) (*dto.TikTokCardResponse, error) {
	card, err := s.repo.GetByID(cardID)
	if err != nil {
		return nil, fmt.Errorf("卡片不存在: %w", err)
	}

	// 真实生成短码
	shortCode := generateRandomCode(8)

	// 如果已有关联短链,先删除
	if card.ShortLinkID != 0 {
		_ = s.db.Delete(&model.ShortLink{}, card.ShortLinkID).Error
	}

	sl := &model.ShortLink{
		ShortCode:   shortCode,
		OriginalURL: fmt.Sprintf("/tiktok/card/%d", card.ID),
		Title:       card.Title,
		Description: card.Description,
		DomainID:    card.DomainPoolID,
	}
	if sl.DomainID == 0 {
		sl.DomainID = 1
	}
	if err := s.db.Create(sl).Error; err != nil {
		return nil, fmt.Errorf("创建短链失败: %w", err)
	}

	card.ShortLinkID = sl.ID
	if _, err := s.repo.Update(card); err != nil {
		return nil, fmt.Errorf("更新卡片短链ID失败: %w", err)
	}

	return s.toResponse(card, shortCode), nil
}

// StatsOverall 获取总体统计
func (s *tiktokCardService) StatsOverall(ctx context.Context) (*dto.TikTokCardStatsOverallResponse, error) {
	totalCards, activeCards, totalViews, popular, err := s.repo.GetOverallStats()
	if err != nil {
		return nil, err
	}

	// 构造每日统计(基于近 7 天活动)
	daily := make([]dto.TikTokCardDailyStat, 0, 7)
	for i := 6; i >= 0; i-- {
		day := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		var dayCount int64
		s.db.Model(&model.TikTokCardActivity{}).
			Where("activity_type = ? AND DATE(created_at) = ?", "view", day).
			Count(&dayCount)
		daily = append(daily, dto.TikTokCardDailyStat{Date: day, ViewCount: dayCount})
	}

	// 最近活动(取最新 10 条)
	var activities []model.TikTokCardActivity
	s.db.Order("created_at DESC").Limit(10).Find(&activities)

	// 构造卡片标题映射
	titleMap := make(map[uint]string, len(popular))
	for _, c := range popular {
		titleMap[c.ID] = c.Title
	}

	recent := make([]dto.TikTokCardActivityItem, 0, len(activities))
	for _, a := range activities {
		recent = append(recent, dto.TikTokCardActivityItem{
			CardTitle: titleMap[a.CardID],
			Action:    a.ActivityType,
			Username:  a.UserID,
			CreatedAt: a.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	popularResp := make([]dto.TikTokCardResponse, 0, len(popular))
	for i := range popular {
		popularResp = append(popularResp, *s.toResponse(&popular[i], ""))
	}

	return &dto.TikTokCardStatsOverallResponse{
		TotalCards:     totalCards,
		ActiveCards:    activeCards,
		TotalViews:     totalViews,
		PopularCards:   popularResp,
		DailyStats:     daily,
		RecentActivity: recent,
	}, nil
}

// Stats 单卡片统计
func (s *tiktokCardService) Stats(ctx context.Context, cardID uint) (*dto.TikTokCardStatsDetailResponse, error) {
	card, activities, err := s.repo.GetCardStats(cardID, 7)
	if err != nil {
		return nil, err
	}

	// 每日浏览统计
	daily := make([]dto.TikTokCardDailyStat, 0, 7)
	for i := 6; i >= 0; i-- {
		day := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		var dayCount int64
		s.db.Model(&model.TikTokCardActivity{}).
			Where("card_id = ? AND activity_type = ? AND DATE(created_at) = ?", cardID, "view", day).
			Count(&dayCount)
		daily = append(daily, dto.TikTokCardDailyStat{Date: day, ViewCount: dayCount})
	}

	recent := make([]dto.TikTokCardActivityItem, 0, len(activities))
	for _, a := range activities {
		recent = append(recent, dto.TikTokCardActivityItem{
			CardTitle: card.Title,
			Action:    a.ActivityType,
			Username:  a.UserID,
			CreatedAt: a.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	return &dto.TikTokCardStatsDetailResponse{
		CardID:         card.ID,
		Title:          card.Title,
		ViewCount:      card.ViewCount,
		DailyStats:     daily,
		RecentActivity: recent,
	}, nil
}

// RecordView 记录一次浏览(递增计数 + 写活动)
func (s *tiktokCardService) RecordView(ctx context.Context, cardID uint, ip, userAgent string) error {
	if err := s.repo.IncrementViewCount(cardID); err != nil {
		return err
	}
	activity := &model.TikTokCardActivity{
		CardID:       cardID,
		ActivityType: "view",
		IPAddress:    ip,
		UserAgent:    userAgent,
		Platform:     "tiktok",
	}
	return s.repo.CreateActivity(activity)
}

// toResponse 转换为响应 DTO
func (s *tiktokCardService) toResponse(card *model.TikTokCard, shortCode string) *dto.TikTokCardResponse {
	shortLinkURL := ""
	if shortCode != "" {
		shortLinkURL = "/s/" + shortCode
	}
	domainPoolID := card.DomainPoolID
	return &dto.TikTokCardResponse{
		ID:           card.ID,
		Title:        card.Title,
		Description:  card.Description,
		ImageURL:     card.ImageURL,
		RedirectURL:  card.RedirectURL,
		DomainPoolID: &domainPoolID,
		ShortLinkURL: shortLinkURL,
		ShortCode:    shortCode,
		Tags:         card.Tags,
		ViewCount:    card.ViewCount,
		LikeCount:    card.LikeCount,
		ShareCount:   card.ShareCount,
		IsActive:     card.IsActive,
		CreatedAt:    card.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    card.UpdatedAt.Format(time.RFC3339),
	}
}

// generateRandomCode 生成短码
func generateRandomCode(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	now := time.Now().UnixNano()
	for i := range b {
		b[i] = charset[now%int64(len(charset))]
		now = now/int64(len(charset)) + 1
	}
	return string(b)
}
