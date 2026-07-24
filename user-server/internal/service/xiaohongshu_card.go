package service

import (
	"context"
	"fmt"
	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/reach/card/template"
	"marketing/internal/repository"
	"time"

	"gorm.io/gorm"
)

// XiaohongshuCardService 小红书卡片服务接口
type XiaohongshuCardService interface {
	Create(ctx context.Context, req *dto.XiaohongshuCardCreateRequest) (*dto.XiaohongshuCardResponse, error)
	Update(ctx context.Context, req *dto.XiaohongshuCardUpdateRequest) (*dto.XiaohongshuCardResponse, error)
	Delete(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*dto.XiaohongshuCardResponse, error)
	GetCardModelByID(ctx context.Context, id uint) (*model.XiaohongshuCard, error)
	GetList(ctx context.Context, req *dto.XiaohongshuCardListRequest) (*dto.XiaohongshuCardListResponse, error)
	GenerateHTMLPage(ctx context.Context, id uint) (string, error)
	GenerateCardChatPage(ctx context.Context, id uint, baseURL string) (string, error)
	GenerateShortLink(ctx context.Context, card *model.XiaohongshuCard) error
	ShareCard(ctx context.Context, id uint, platform string) (*dto.XiaohongshuCardResponse, error)
}

// xiaohongshuCardService 小红书卡片服务实现
type xiaohongshuCardService struct {
	repo              repository.XiaohongshuCardRepository
	templateService   *template.TemplateService
	shortLinkService  ShortLinkService
	domainPoolService DomainPoolService
}

// NewXiaohongshuCardService 创建小红书卡片服务实例
func NewXiaohongshuCardService(db *gorm.DB) XiaohongshuCardService {
	return &xiaohongshuCardService{
		repo:              repository.NewXiaohongshuCardRepository(db),
		templateService:   template.NewTemplateService("internal/template"),
		shortLinkService:  NewShortLinkService(db),
		domainPoolService: NewDomainPoolService(db),
	}
}

// Create 创建小红书卡片
func (s *xiaohongshuCardService) Create(ctx context.Context, req *dto.XiaohongshuCardCreateRequest) (*dto.XiaohongshuCardResponse, error) {
	// 如果没有提供跳转链接，设置默认值
	redirectURL := req.RedirectURL
	if redirectURL == "" {
		redirectURL = "https://www.xiaohongshu.com"
	}

	card := &model.XiaohongshuCard{
		Title:        req.Title,
		Description:  req.Description,
		ImageURL:     req.ImageURL,
		RedirectURL:  redirectURL,
		DomainPoolID: req.DomainPoolID,
		Tags:         req.Tags,
		ViewCount:    0,
		IsActive:     req.IsActive,
	}

	createdCard, err := s.repo.Create(ctx, card)
	if err != nil {
		return nil, err
	}

	// 生成短链
	err = s.GenerateShortLink(ctx, createdCard)
	if err != nil {
		return nil, err
	}

	// 重新获取卡片信息以获取短链
	card, err = s.repo.GetByID(ctx, createdCard.ID)
	if err != nil {
		return nil, err
	}

	// 获取短链信息
	shortCode := ""
	if card.ShortLinkID != nil {
		shortLink, err := s.shortLinkService.GetByID(context.Background(), *card.ShortLinkID)
		if err == nil {
			shortCode = shortLink.ShortCode
		}
	}

	return s.toResponseWithShortLink(ctx, card, shortCode), nil
}

// Update 更新小红书卡片
func (s *xiaohongshuCardService) Update(ctx context.Context, req *dto.XiaohongshuCardUpdateRequest) (*dto.XiaohongshuCardResponse, error) {
	card, err := s.repo.GetByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}

	// 保存原始域名池ID用于比较
	originalDomainPoolID := card.DomainPoolID

	// 如果没有提供跳转链接，设置默认值
	redirectURL := req.RedirectURL
	if redirectURL == "" {
		redirectURL = "https://www.xiaohongshu.com"
	}

	// 更新字段
	card.Title = req.Title
	card.Description = req.Description
	card.ImageURL = req.ImageURL
	card.RedirectURL = redirectURL
	card.DomainPoolID = req.DomainPoolID
	card.Tags = req.Tags
	card.IsActive = req.IsActive

	updatedCard, err := s.repo.Update(ctx, card)
	if err != nil {
		return nil, err
	}

	// 如果域名池ID发生变化，重新生成短链
	if originalDomainPoolID != req.DomainPoolID {
		err = s.GenerateShortLink(ctx, updatedCard)
		if err != nil {
			return nil, err
		}

		// 重新获取卡片信息以获取短链
		updatedCard, err = s.repo.GetByID(ctx, updatedCard.ID)
		if err != nil {
			return nil, err
		}
	}

	// 获取短链信息
	shortCode := ""
	if updatedCard.ShortLinkID != nil {
		shortLink, err := s.shortLinkService.GetByID(context.Background(), *updatedCard.ShortLinkID)
		if err == nil {
			shortCode = shortLink.ShortCode
		}
	}

	return s.toResponseWithShortLink(ctx, updatedCard, shortCode), nil
}

// Delete 删除小红书卡片
func (s *xiaohongshuCardService) Delete(ctx context.Context, id uint) error {
	// 获取卡片信息，以便删除关联的短链
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// 如果有关联的短链，删除短链
	if card.ShortLinkID != nil {
		_ = s.shortLinkService.Delete(context.Background(), *card.ShortLinkID)
	}

	// 删除卡片
	return s.repo.Delete(ctx, id)
}

// GetByID 根据ID获取小红书卡片
func (s *xiaohongshuCardService) GetByID(ctx context.Context, id uint) (*dto.XiaohongshuCardResponse, error) {
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return s.toResponse(ctx, card), nil
}

// GetCardModelByID 根据ID获取小红书卡片模型
func (s *xiaohongshuCardService) GetCardModelByID(ctx context.Context, id uint) (*model.XiaohongshuCard, error) {
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return card, nil
}

// GetList 获取小红书卡片列表
func (s *xiaohongshuCardService) GetList(ctx context.Context, req *dto.XiaohongshuCardListRequest) (*dto.XiaohongshuCardListResponse, error) {
	filter := repository.CardListFilter{Page: req.Page, PageSize: req.PageSize, Keyword: req.Keyword, IsActive: req.IsActive}
	cards, total, err := s.repo.GetList(ctx, filter)
	if err != nil {
		return nil, err
	}

	// 转换为响应格式
	responses := make([]dto.XiaohongshuCardResponse, len(cards))
	for i, card := range cards {
		// 获取短链信息
		shortCode := ""
		shortLinkURL := ""
		if card.ShortLinkID != nil {
			shortLink, err := s.shortLinkService.GetByID(context.Background(), *card.ShortLinkID)
			if err == nil {
				shortCode = shortLink.ShortCode
				shortLinkURL = "/s/" + shortCode
			}
		}

		responses[i] = dto.XiaohongshuCardResponse{
			ID:           card.ID,
			Title:        card.Title,
			Description:  card.Description,
			ImageURL:     card.ImageURL,
			RedirectURL:  card.RedirectURL,
			DomainPoolID: card.DomainPoolID,
			ShortLinkURL: shortLinkURL,
			ShortCode:    shortCode,
			Tags:         card.Tags,
			ViewCount:    card.ViewCount,
			IsActive:     card.IsActive,
			CreatedAt:    card.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:    card.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	return &dto.XiaohongshuCardListResponse{
		List:  responses,
		Total: total,
	}, nil
}

// ShareCard 分享小红书卡片
func (s *xiaohongshuCardService) ShareCard(ctx context.Context, id uint, platform string) (*dto.XiaohongshuCardResponse, error) {
	// 获取卡片信息
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 记录活动
	activity := &model.XiaohongshuCardActivity{
		CardID:       id,
		ActivityType: "share",
		Content:      platform,
	}
	_ = s.repo.CreateActivity(ctx, activity)

	return s.toResponse(ctx, card), nil
}

// recordActivity 记录卡片活动
func (s *xiaohongshuCardService) recordActivity(ctx context.Context, cardID uint, activityType string) error {
	activity := &model.XiaohongshuCardActivity{
		CardID:       cardID,
		ActivityType: activityType,
		CreatedAt:    time.Now(),
	}

	return s.repo.CreateActivity(ctx, activity)
}

// GenerateHTMLPage 生成小红书卡片HTML页面
func (s *xiaohongshuCardService) GenerateHTMLPage(ctx context.Context, id uint) (string, error) {
	// 获取卡片信息
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}

	// 使用模板服务生成HTML页面
	data := &template.CardTemplateData{
		Title:       card.Title,
		Description: card.Description,
		ImageURL:    card.ImageURL,
		RedirectURL: card.RedirectURL,
	}

	html, err := s.templateService.GenerateXiaohongshuCardPage(data)
	if err != nil {
		return "", err
	}

	return html, nil
}

// GenerateCardChatPage 生成小红书卡片聊天页（统一模板，含联系客服按钮）
func (s *xiaohongshuCardService) GenerateCardChatPage(ctx context.Context, id uint, baseURL string) (string, error) {
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	return s.templateService.RenderCardChatPage("xiaohongshu", card.ID, card.Title, card.Description, card.ImageURL, card.Tags, baseURL)
}

// GenerateShortLink 为小红书卡片生成短链
func (s *xiaohongshuCardService) GenerateShortLink(ctx context.Context, card *model.XiaohongshuCard) error {
	// 如果已经有短链，先删除旧的短链
	if card.ShortLinkID != nil {
		// 可以选择删除旧短链或保留，这里我们保留旧短链但不再使用
		// 如果需要删除旧短链，可以调用 s.shortLinkService.Delete(*card.ShortLinkID)
	}

	// 获取域名池信息
	var domainID uint = 0 // 默认域名ID为0
	if card.DomainPoolID != nil {
		// 这里需要根据域名池信息找到对应的域名ID
		// 假设域名池ID和域名ID相同，实际可能需要查询映射关系
		domainID = *card.DomainPoolID
	}

	// 生成短码
	shortCodeReq := &dto.GenerateShortCodeRequest{
		Length: 6,
	}
	shortCodeResp, err := s.shortLinkService.GenerateShortCode(context.Background(), shortCodeReq)
	if err != nil {
		return err
	}

	// 创建短链
	createReq := &dto.CreateShortLinkRequest{
		ShortCode:   shortCodeResp.ShortCode,
		OriginalURL: fmt.Sprintf("/xiaohongshu/card/%d", card.ID), // 指向小红书卡片页面
		Title:       card.Title,
		Description: card.Description,
		DomainID:    domainID, // 使用域名池ID作为域名ID
	}

	shortLinkResp, err := s.shortLinkService.Create(ctx, createReq)
	if err != nil {
		return err
	}

	// 更新卡片的短链ID
	err = s.repo.UpdateShortLinkID(ctx, card.ID, &shortLinkResp.ID)
	if err != nil {
		return err
	}

	return nil
}

// toResponse 将模型转换为响应DTO
func (s *xiaohongshuCardService) toResponse(ctx context.Context, card *model.XiaohongshuCard) *dto.XiaohongshuCardResponse {
	// 获取短链信息
	shortCode := ""
	shortLinkURL := ""
	if card.ShortLinkID != nil {
		shortLink, err := s.shortLinkService.GetByID(ctx, *card.ShortLinkID)
		if err == nil {
			shortCode = shortLink.ShortCode
			shortLinkURL = "/s/" + shortCode
		}
	}

	return &dto.XiaohongshuCardResponse{
		ID:           card.ID,
		Title:        card.Title,
		Description:  card.Description,
		ImageURL:     card.ImageURL,
		RedirectURL:  card.RedirectURL,
		DomainPoolID: card.DomainPoolID,
		ShortLinkURL: shortLinkURL,
		ShortCode:    shortCode,
		Tags:         card.Tags,
		ViewCount:    card.ViewCount,
		IsActive:     card.IsActive,
		CreatedAt:    card.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:    card.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}

// toResponseWithShortLink 将模型转换为响应DTO，包含短链信息
func (s *xiaohongshuCardService) toResponseWithShortLink(ctx context.Context, card *model.XiaohongshuCard, shortCode string) *dto.XiaohongshuCardResponse {
	shortLinkURL := ""
	if shortCode != "" {
		shortLinkURL = "/s/" + shortCode
	}

	return &dto.XiaohongshuCardResponse{
		ID:           card.ID,
		Title:        card.Title,
		Description:  card.Description,
		ImageURL:     card.ImageURL,
		RedirectURL:  card.RedirectURL,
		DomainPoolID: card.DomainPoolID,
		ShortLinkURL: shortLinkURL,
		ShortCode:    shortCode,
		Tags:         card.Tags,
		ViewCount:    card.ViewCount,
		IsActive:     card.IsActive,
		CreatedAt:    card.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:    card.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}
