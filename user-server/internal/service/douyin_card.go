package service

import (
	"context"
	"fmt"
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/reach/card/template"
	"hivemtk-user/internal/repository"
	"time"

	"gorm.io/gorm"
)

// DouyinCardService 抖音卡片服务接口
type DouyinCardService interface {
	Create(ctx context.Context, req *dto.DouyinCardCreateRequest) (*dto.DouyinCardResponse, error)
	Update(ctx context.Context, req *dto.DouyinCardUpdateRequest) (*dto.DouyinCardResponse, error)
	Delete(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*dto.DouyinCardResponse, error)
	GetByIDWithRefresh(ctx context.Context, id uint) (*dto.DouyinCardResponse, error)
	GetCardModelByID(ctx context.Context, id uint) (*model.DouyinCard, error)
	GetList(ctx context.Context, req *dto.DouyinCardListRequest) (*dto.DouyinCardListResponse, error)
	ShareCard(ctx context.Context, id uint, platform string) error
	GenerateHTMLPage(ctx context.Context, id uint) (string, error)
	GenerateCardChatPage(ctx context.Context, id uint, baseURL string) (string, error)
	GenerateShortLink(ctx context.Context, card *model.DouyinCard) error
}

// douyinCardService 抖音卡片服务实现
type douyinCardService struct {
	repo              repository.DouyinCardRepository
	statsService      DouyinCardStatsService
	shortLinkService  ShortLinkService
	domainPoolService DomainPoolService
	templateService   *template.TemplateService
}

// NewDouyinCardService 创建抖音卡片服务
func NewDouyinCardService(db any) DouyinCardService {
	gormDB := db.(*gorm.DB)
	return &douyinCardService{
		repo:              repository.NewDouyinCardRepository(gormDB),
		statsService:      NewDouyinCardStatsService(gormDB),
		shortLinkService:  NewShortLinkService(gormDB),
		domainPoolService: NewDomainPoolService(gormDB),
		templateService:   template.NewTemplateService("internal/template"),
	}
}

// Create 创建抖音卡片
func (s *douyinCardService) Create(ctx context.Context, req *dto.DouyinCardCreateRequest) (*dto.DouyinCardResponse, error) {
	redirectURL := req.RedirectURL

	if redirectURL == "" {
		redirectURL = "https://www.douyin.com"
	}

	card := &model.DouyinCard{
		Title:        req.Title,
		Description:  req.Description,
		ImageURL:     req.ImageURL,
		RedirectURL:  redirectURL,
		DomainPoolID: req.DomainPoolID,
		Tags:         req.Tags,
		IsActive:     req.IsActive,
	}

	createdCard, err := s.repo.Create(ctx, card)
	if err != nil {
		return nil, err
	}

	if err := s.GenerateShortLink(ctx, createdCard); err != nil {
		logger.Errorf("警告：生成短链失败：%v", err)
	}

	shortCode := ""
	if createdCard.ShortLinkID != 0 {
		shortLink, err := s.shortLinkService.GetByID(ctx, createdCard.ShortLinkID)
		if err == nil {
			shortCode = shortLink.ShortCode
		}
	}

	return s.toResponseWithShortLink(ctx, createdCard, shortCode), nil
}

// Update 更新抖音卡片
func (s *douyinCardService) Update(ctx context.Context, req *dto.DouyinCardUpdateRequest) (*dto.DouyinCardResponse, error) {
	card, err := s.repo.GetByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}

	originalDomainPoolID := card.DomainPoolID

	redirectURL := req.RedirectURL

	if redirectURL == "" {
		redirectURL = "https://www.douyin.com"
	}

	card.Title = req.Title
	card.Description = req.Description
	card.ImageURL = req.ImageURL
	card.RedirectURL = redirectURL
	card.DomainPoolID = req.DomainPoolID
	card.Tags = req.Tags
	card.ViewCount = req.ViewCount
	card.IsActive = req.IsActive

	updatedCard, err := s.repo.Update(ctx, card)
	if err != nil {
		return nil, err
	}

	if originalDomainPoolID != req.DomainPoolID {
		if err := s.GenerateShortLink(ctx, updatedCard); err != nil {
			return nil, err
		}
	}

	shortCode := ""
	if updatedCard.ShortLinkID != 0 {
		shortLink, err := s.shortLinkService.GetByID(ctx, updatedCard.ShortLinkID)
		if err == nil {
			shortCode = shortLink.ShortCode
		}
	}

	return s.toResponseWithShortLink(ctx, updatedCard, shortCode), nil
}

// Delete 删除抖音卡片
func (s *douyinCardService) Delete(ctx context.Context, id uint) error {
	return s.repo.Delete(ctx, id)
}

// GetByID 根据ID获取抖音卡片
func (s *douyinCardService) GetByID(ctx context.Context, id uint) (*dto.DouyinCardResponse, error) {
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	shortCode := ""
	if card.ShortLinkID != 0 {
		shortLink, err := s.shortLinkService.GetByID(ctx, card.ShortLinkID)
		if err == nil {
			shortCode = shortLink.ShortCode
		}
	}

	return s.toResponseWithShortLink(ctx, card, shortCode), nil
}

// GetByIDWithRefresh 根据ID获取抖音卡片，强制刷新缓存
func (s *douyinCardService) GetByIDWithRefresh(ctx context.Context, id uint) (*dto.DouyinCardResponse, error) {
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	shortCode := ""
	if card.ShortLinkID != 0 {
		shortLink, err := s.shortLinkService.GetByID(ctx, card.ShortLinkID)
		if err == nil {
			shortCode = shortLink.ShortCode
		}
	}

	return s.toResponseWithShortLink(ctx, card, shortCode), nil
}

// GetCardModelByID 根据ID获取抖音卡片模型
func (s *douyinCardService) GetCardModelByID(ctx context.Context, id uint) (*model.DouyinCard, error) {
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return card, nil
}

// GetList 获取抖音卡片列表
func (s *douyinCardService) GetList(ctx context.Context, req *dto.DouyinCardListRequest) (*dto.DouyinCardListResponse, error) {
	filter := repository.CardListFilter{Page: req.Page, PageSize: req.PageSize, Keyword: req.Keyword, IsActive: req.IsActive}
	cards, total, err := s.repo.GetList(ctx, filter)
	if err != nil {
		return nil, err
	}

	var responses []dto.DouyinCardResponse
	for _, card := range cards {
		shortCode := ""
		if card.ShortLinkID != 0 {
			shortLink, err := s.shortLinkService.GetByID(ctx, card.ShortLinkID)
			if err == nil {
				shortCode = shortLink.ShortCode
			}
		}
		responses = append(responses, *s.toResponseWithShortLink(ctx, &card, shortCode))
	}

	return &dto.DouyinCardListResponse{
		List:  responses,
		Total: total,
	}, nil
}

// ShareCard 分享抖音卡片
func (s *douyinCardService) ShareCard(ctx context.Context, id uint, platform string) error {
	err := s.repo.IncrementShareCount(ctx, id)
	if err != nil {
		return err
	}

	_ = s.statsService.RecordActivity(ctx, id, 0, "share", platform, "", "")

	return nil
}

// GenerateHTMLPage 生成抖音卡片HTML页面
func (s *douyinCardService) GenerateHTMLPage(ctx context.Context, id uint) (string, error) {
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}

	data := &template.CardTemplateData{
		Title:       card.Title,
		Description: card.Description,
		ImageURL:    card.ImageURL,
		RedirectURL: card.RedirectURL,
	}

	html, err := s.templateService.GenerateDouyinCardPage(data)
	if err != nil {
		return "", err
	}

	return html, nil
}

// GenerateCardChatPage 生成抖音卡片聊天页（统一模板，含联系客服按钮）
func (s *douyinCardService) GenerateCardChatPage(ctx context.Context, id uint, baseURL string) (string, error) {
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	return s.templateService.RenderCardChatPage("douyin", card.ID, card.Title, card.Description, card.ImageURL, card.Tags, baseURL)
}

// toResponse 将模型转换为响应DTO
func (s *douyinCardService) toResponse(ctx context.Context, card *model.DouyinCard) *dto.DouyinCardResponse {
	return &dto.DouyinCardResponse{
		ID:           card.ID,
		Title:        card.Title,
		Description:  card.Description,
		ImageURL:     card.ImageURL,
		RedirectURL:  card.RedirectURL,
		DomainPoolID: &card.DomainPoolID, 
		Tags:         card.Tags,
		ViewCount:    card.ViewCount,
		IsActive:     card.IsActive,
		CreatedAt:    card.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    card.UpdatedAt.Format(time.RFC3339),
	}
}

// toResponseWithShortLink 将模型转换为响应DTO，包含短链信息
func (s *douyinCardService) toResponseWithShortLink(ctx context.Context, card *model.DouyinCard, shortCode string) *dto.DouyinCardResponse {
	shortLinkURL := ""
	if shortCode != "" {
		shortLinkURL = "/s/" + shortCode
	}

	return &dto.DouyinCardResponse{
		ID:           card.ID,
		Title:        card.Title,
		Description:  card.Description,
		ImageURL:     card.ImageURL,
		RedirectURL:  card.RedirectURL,
		DomainPoolID: &card.DomainPoolID, 
		ShortLinkURL: shortLinkURL,
		ShortCode:    shortCode,
		Tags:         card.Tags,
		ViewCount:    card.ViewCount,
		IsActive:     card.IsActive,
		CreatedAt:    card.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    card.UpdatedAt.Format(time.RFC3339),
	}
}

// GenerateShortLink 为抖音卡片生成短链
func (s *douyinCardService) GenerateShortLink(ctx context.Context, card *model.DouyinCard) error {
	if card.ShortLinkID != 0 {
		_ = s.shortLinkService.Delete(ctx, card.ShortLinkID)
	}

	generateReq := &dto.GenerateShortCodeRequest{
		Length: 6,
	}
	generateResp, err := s.shortLinkService.GenerateShortCode(ctx, generateReq)
	if err != nil {
		return fmt.Errorf("生成短码失败: %v", err)
	}

	logger.Infof("生成的短码: %s", generateResp.ShortCode)

	// 获取域名池域名：通过 DomainPoolService 解析真实域名，避免硬编码 =1
	var domainID uint
	if card.DomainPoolID != 0 {
		if pool, perr := s.domainPoolService.GetByID(ctx, int(card.DomainPoolID)); perr == nil && pool != nil {
			domainID = uint(pool.ID)
		}
	}

	shortLinkReq := &dto.CreateShortLinkRequest{
		ShortCode:   generateResp.ShortCode,
		OriginalURL: fmt.Sprintf("/douyin/card/%d", card.ID), 
		Title:       card.Title,
		Description: card.Description,
		DomainID:    domainID, 
	}

	shortLinkResp, err := s.shortLinkService.Create(ctx, shortLinkReq)
	if err != nil {
		return fmt.Errorf("创建短链失败: %v", err)
	}

	logger.Infof("创建的短链ID: %d", shortLinkResp.ID)

	card.ShortLinkID = shortLinkResp.ID
	_, err = s.repo.Update(ctx, card)
	if err != nil {
		return fmt.Errorf("更新卡片短链ID失败: %v", err)
	}

	logger.Infof("更新卡片短链ID成功: %d", card.ShortLinkID)

	updatedCard, err := s.repo.GetByID(ctx, card.ID)
	if err != nil {
		return fmt.Errorf("获取更新后的卡片失败: %v", err)
	}

	if updatedCard.ShortLinkID != shortLinkResp.ID {
		return fmt.Errorf("短链ID更新失败")
	}

	logger.Infof("验证更新后的卡片短链ID: %d", updatedCard.ShortLinkID)

	return nil
}

