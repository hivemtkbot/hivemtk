package service

import (
	"context"
	"fmt"
	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/reach/card/template"
	"marketing/internal/repository"
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
	// 类型断言将interface{}转换为*gorm.DB
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
	// 处理跳转链接
	redirectURL := req.RedirectURL

	// 如果没有提供跳转链接，设置默认值
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

	createdCard, err := s.repo.Create(card)
	if err != nil {
		return nil, err
	}

	// 生成短码和短链（可选功能，失败不影响卡片创建）
	if err := s.GenerateShortLink(ctx, createdCard); err != nil {
		// 记录日志但不返回错误，允许卡片创建成功
		logger.Errorf("警告：生成短链失败：%v", err)
	}

	// 获取短链信息
	shortCode := ""
	if createdCard.ShortLinkID != 0 {
		shortLink, err := s.shortLinkService.GetByID(createdCard.ShortLinkID)
		if err == nil {
			shortCode = shortLink.ShortCode
		}
	}

	return s.toResponseWithShortLink(createdCard, shortCode), nil
}

// Update 更新抖音卡片
func (s *douyinCardService) Update(ctx context.Context, req *dto.DouyinCardUpdateRequest) (*dto.DouyinCardResponse, error) {
	card, err := s.repo.GetByID(req.ID)
	if err != nil {
		return nil, err
	}

	// 保存原始域名池ID，用于比较
	originalDomainPoolID := card.DomainPoolID

	// 处理跳转链接
	redirectURL := req.RedirectURL

	// 如果没有提供跳转链接，设置默认值
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

	updatedCard, err := s.repo.Update(card)
	if err != nil {
		return nil, err
	}

	// 如果域名池ID发生变化，重新生成短链
	if originalDomainPoolID != req.DomainPoolID {
		if err := s.GenerateShortLink(ctx, updatedCard); err != nil {
			return nil, err
		}
	}

	// 获取短链信息
	shortCode := ""
	if updatedCard.ShortLinkID != 0 {
		shortLink, err := s.shortLinkService.GetByID(updatedCard.ShortLinkID)
		if err == nil {
			shortCode = shortLink.ShortCode
		}
	}

	return s.toResponseWithShortLink(updatedCard, shortCode), nil
}

// Delete 删除抖音卡片
func (s *douyinCardService) Delete(ctx context.Context, id uint) error {
	return s.repo.Delete(id)
}

// GetByID 根据ID获取抖音卡片
func (s *douyinCardService) GetByID(ctx context.Context, id uint) (*dto.DouyinCardResponse, error) {
	card, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	// 获取短链信息
	shortCode := ""
	if card.ShortLinkID != 0 {
		shortLink, err := s.shortLinkService.GetByID(card.ShortLinkID)
		if err == nil {
			shortCode = shortLink.ShortCode
		}
	}

	return s.toResponseWithShortLink(card, shortCode), nil
}

// GetByIDWithRefresh 根据ID获取抖音卡片，强制刷新缓存
func (s *douyinCardService) GetByIDWithRefresh(ctx context.Context, id uint) (*dto.DouyinCardResponse, error) {
	// 直接从数据库获取最新的卡片信息，不使用缓存
	card, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	// 获取短链信息
	shortCode := ""
	if card.ShortLinkID != 0 {
		shortLink, err := s.shortLinkService.GetByID(card.ShortLinkID)
		if err == nil {
			shortCode = shortLink.ShortCode
		}
	}

	return s.toResponseWithShortLink(card, shortCode), nil
}

// GetCardModelByID 根据ID获取抖音卡片模型
func (s *douyinCardService) GetCardModelByID(ctx context.Context, id uint) (*model.DouyinCard, error) {
	card, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	return card, nil
}

// GetList 获取抖音卡片列表
func (s *douyinCardService) GetList(ctx context.Context, req *dto.DouyinCardListRequest) (*dto.DouyinCardListResponse, error) {
	cards, total, err := s.repo.GetList(req)
	if err != nil {
		return nil, err
	}

	var responses []dto.DouyinCardResponse
	for _, card := range cards {
		// 获取短链信息
		shortCode := ""
		if card.ShortLinkID != 0 {
			shortLink, err := s.shortLinkService.GetByID(card.ShortLinkID)
			if err == nil {
				shortCode = shortLink.ShortCode
			}
		}
		responses = append(responses, *s.toResponseWithShortLink(&card, shortCode))
	}

	return &dto.DouyinCardListResponse{
		List:  responses,
		Total: total,
	}, nil
}

// ShareCard 分享抖音卡片
func (s *douyinCardService) ShareCard(ctx context.Context, id uint, platform string) error {
	err := s.repo.IncrementShareCount(id)
	if err != nil {
		return err
	}

	// 记录活动
	_ = s.statsService.RecordActivity(id, 0, "share", platform, "", "")

	return nil
}

// GenerateHTMLPage 生成抖音卡片HTML页面
func (s *douyinCardService) GenerateHTMLPage(ctx context.Context, id uint) (string, error) {
	card, err := s.repo.GetByID(id)
	if err != nil {
		return "", err
	}

	// 准备模板数据
	data := &template.CardTemplateData{
		Title:       card.Title,
		Description: card.Description,
		ImageURL:    card.ImageURL,
		RedirectURL: card.RedirectURL,
	}

	// 生成HTML页面
	html, err := s.templateService.GenerateDouyinCardPage(data)
	if err != nil {
		return "", err
	}

	return html, nil
}

// GenerateCardChatPage 生成抖音卡片聊天页（统一模板，含联系客服按钮）
func (s *douyinCardService) GenerateCardChatPage(ctx context.Context, id uint, baseURL string) (string, error) {
	card, err := s.repo.GetByID(id)
	if err != nil {
		return "", err
	}
	return s.templateService.RenderCardChatPage("douyin", card.ID, card.Title, card.Description, card.ImageURL, card.Tags, baseURL)
}

// toResponse 将模型转换为响应DTO
func (s *douyinCardService) toResponse(card *model.DouyinCard) *dto.DouyinCardResponse {
	return &dto.DouyinCardResponse{
		ID:           card.ID,
		Title:        card.Title,
		Description:  card.Description,
		ImageURL:     card.ImageURL,
		RedirectURL:  card.RedirectURL,
		DomainPoolID: &card.DomainPoolID, // 转换为指针类型以保持API兼容性
		Tags:         card.Tags,
		ViewCount:    card.ViewCount,
		IsActive:     card.IsActive,
		CreatedAt:    card.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    card.UpdatedAt.Format(time.RFC3339),
	}
}

// toResponseWithShortLink 将模型转换为响应DTO，包含短链信息
func (s *douyinCardService) toResponseWithShortLink(card *model.DouyinCard, shortCode string) *dto.DouyinCardResponse {
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
		DomainPoolID: &card.DomainPoolID, // 转换为指针类型以保持API兼容性
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
	// 检查是否已有短链
	if card.ShortLinkID != 0 {
		// 删除已有短链
		_ = s.shortLinkService.Delete(card.ShortLinkID)
	}

	// 生成短码
	generateReq := &dto.GenerateShortCodeRequest{
		Length: 6,
	}
	generateResp, err := s.shortLinkService.GenerateShortCode(generateReq)
	if err != nil {
		return fmt.Errorf("生成短码失败: %v", err)
	}

	logger.Infof("生成的短码: %s", generateResp.ShortCode)

	// 获取域名池域名
	var domainID uint = 1 // 默认使用ID为1的域名
	if card.DomainPoolID != 0 {
		// 这里需要根据域名池信息找到对应的域名ID
		// 假设域名池ID和域名ID相同，实际可能需要查询映射关系
		domainID = card.DomainPoolID
	}

	// 创建短链
	shortLinkReq := &dto.CreateShortLinkRequest{
		ShortCode:   generateResp.ShortCode,
		OriginalURL: fmt.Sprintf("/douyin/card/%d", card.ID), // 指向抖音卡片页面
		Title:       card.Title,
		Description: card.Description,
		DomainID:    domainID, // 使用域名池ID作为域名ID
	}

	shortLinkResp, err := s.shortLinkService.Create(shortLinkReq)
	if err != nil {
		return fmt.Errorf("创建短链失败: %v", err)
	}

	logger.Infof("创建的短链ID: %d", shortLinkResp.ID)

	// 更新卡片的短链ID
	card.ShortLinkID = shortLinkResp.ID
	_, err = s.repo.Update(card)
	if err != nil {
		return fmt.Errorf("更新卡片短链ID失败: %v", err)
	}

	logger.Infof("更新卡片短链ID成功: %d", card.ShortLinkID)

	// 确保更新后的数据被正确保存，刷新数据库连接
	// 这是为了解决可能的缓存问题
	updatedCard, err := s.repo.GetByID(card.ID)
	if err != nil {
		return fmt.Errorf("获取更新后的卡片失败: %v", err)
	}

	// 验证短链ID是否正确更新
	if updatedCard.ShortLinkID != shortLinkResp.ID {
		return fmt.Errorf("短链ID更新失败")
	}

	logger.Infof("验证更新后的卡片短链ID: %d", updatedCard.ShortLinkID)

	return nil
}
