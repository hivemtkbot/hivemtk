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

// KuaishouCardService 快手卡片服务接口
type KuaishouCardService interface {
	Create(ctx context.Context, req *dto.KuaishouCardCreateRequest) (*dto.KuaishouCardResponse, error)
	Update(ctx context.Context, req *dto.KuaishouCardUpdateRequest) (*dto.KuaishouCardResponse, error)
	Delete(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*dto.KuaishouCardResponse, error)
	GetByIDWithRefresh(ctx context.Context, id uint) (*dto.KuaishouCardResponse, error)
	GetCardModelByID(ctx context.Context, id uint) (*model.KuaishouCard, error)
	GetList(ctx context.Context, req *dto.KuaishouCardListRequest) (*dto.KuaishouCardListResponse, error)
	ViewCard(ctx context.Context, id uint) error
	LikeCard(ctx context.Context, id uint) error
	ShareCard(ctx context.Context, id uint, platform string) (*dto.KuaishouCardResponse, error)
	GenerateHTMLPage(ctx context.Context, id uint) (string, error)
	GenerateCardChatPage(ctx context.Context, id uint, baseURL string) (string, error)
	GenerateShortLink(ctx context.Context, card *model.KuaishouCard) error
}

// kuaishouCardService 快手卡片服务实现
type kuaishouCardService struct {
	repo              repository.KuaishouCardRepository
	shortLinkService  ShortLinkService
	domainPoolService DomainPoolService
	templateService   *template.TemplateService
}

// NewKuaishouCardService 创建快手卡片服务实例
func NewKuaishouCardService(db any) KuaishouCardService {
	// 类型断言将interface{}转换为*gorm.DB
	gormDB := db.(*gorm.DB)
	return &kuaishouCardService{
		repo:              repository.NewKuaishouCardRepository(gormDB),
		shortLinkService:  NewShortLinkService(gormDB),
		domainPoolService: NewDomainPoolService(gormDB),
		templateService:   template.NewTemplateService("internal/template"),
	}
}

// Create 创建快手卡片
func (s *kuaishouCardService) Create(ctx context.Context, req *dto.KuaishouCardCreateRequest) (*dto.KuaishouCardResponse, error) {
	// 处理跳转链接
	redirectURL := req.RedirectURL
	// 如果没有提供跳转链接，设置默认值
	if redirectURL == "" {
		redirectURL = "https://www.kuaishou.com"
	}

	// 如果没有提供跳转链接，设置默认值
	if redirectURL == "" {
		redirectURL = "https://www.kuaishou.com"
	}

	card := &model.KuaishouCard{
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

	// 生成短链（可选功能：失败时仅记日志，不影响主流程）
	if err := s.GenerateShortLink(ctx, createdCard); err != nil {
		logger.Errorf("警告：生成短链失败：%v", err)
	}

	// 重新获取卡片信息以获取短链
	card, err = s.repo.GetByID(ctx, createdCard.ID)
	if err != nil {
		return nil, err
	}

	// 获取短链信息
	shortCode := ""
	if card.ShortLinkID != nil && *card.ShortLinkID != 0 {
		shortLink, err := s.shortLinkService.GetByID(ctx, *card.ShortLinkID)
		if err == nil {
			shortCode = shortLink.ShortCode
		}
	}

	return s.toResponseWithShortLink(ctx, card, shortCode), nil
}

// Update 更新快手卡片
func (s *kuaishouCardService) Update(ctx context.Context, req *dto.KuaishouCardUpdateRequest) (*dto.KuaishouCardResponse, error) {
	// 获取现有卡片
	card, err := s.repo.GetByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}

	// 处理跳转链接
	redirectURL := req.RedirectURL
	// 如果没有提供跳转链接，设置默认值
	if redirectURL == "" {
		redirectURL = "https://www.kuaishou.com"
	}

	// 更新字段
	card.Title = req.Title
	card.Description = req.Description
	card.ImageURL = req.ImageURL
	card.RedirectURL = redirectURL
	card.DomainPoolID = req.DomainPoolID
	card.Tags = req.Tags
	card.LikeCount = req.LikeCount
	card.ShareCount = req.ShareCount
	card.IsActive = req.IsActive

	updatedCard, err := s.repo.Update(ctx, card)
	if err != nil {
		return nil, err
	}

	// 每次更新都重新生成短链（可选功能：失败时仅记日志，不影响主流程）
	if err := s.GenerateShortLink(ctx, updatedCard); err != nil {
		logger.Errorf("警告：生成短链失败：%v", err)
	}

	// 重新获取卡片信息以获取短链
	card, err = s.repo.GetByID(ctx, updatedCard.ID)
	if err != nil {
		return nil, err
	}

	// 获取短链信息
	shortCode := ""
	if card.ShortLinkID != nil && *card.ShortLinkID != 0 {
		shortLink, err := s.shortLinkService.GetByID(ctx, *card.ShortLinkID)
		if err == nil {
			shortCode = shortLink.ShortCode
		}
	}

	return s.toResponseWithShortLink(ctx, card, shortCode), nil
}

// Delete 删除快手卡片
func (s *kuaishouCardService) Delete(ctx context.Context, id uint) error {
	// 检查卡片是否存在
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// 如果存在短链，删除短链
	if card.ShortLinkID != nil && *card.ShortLinkID != 0 {
		_ = s.shortLinkService.Delete(ctx, *card.ShortLinkID)
	}

	return s.repo.Delete(ctx, id)
}

// GetByID 根据ID获取快手卡片
func (s *kuaishouCardService) GetByID(ctx context.Context, id uint) (*dto.KuaishouCardResponse, error) {
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 获取短链信息
	shortCode := ""
	if card.ShortLinkID != nil && *card.ShortLinkID != 0 {
		shortLink, err := s.shortLinkService.GetByID(ctx, *card.ShortLinkID)
		if err == nil {
			shortCode = shortLink.ShortCode
		}
	}

	return s.toResponseWithShortLink(ctx, card, shortCode), nil
}

// GetByIDWithRefresh 获取并刷新卡片信息
func (s *kuaishouCardService) GetByIDWithRefresh(ctx context.Context, id uint) (*dto.KuaishouCardResponse, error) {
	// 直接从数据库获取最新的卡片信息，不使用缓存
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 获取短链信息
	shortCode := ""
	if card.ShortLinkID != nil && *card.ShortLinkID != 0 {
		shortLink, err := s.shortLinkService.GetByID(ctx, *card.ShortLinkID)
		if err == nil {
			shortCode = shortLink.ShortCode
		}
	}

	return s.toResponseWithShortLink(ctx, card, shortCode), nil
}

// GetCardModelByID 获取卡片模型
func (s *kuaishouCardService) GetCardModelByID(ctx context.Context, id uint) (*model.KuaishouCard, error) {
	return s.repo.GetByID(ctx, id)
}

// GetList 获取快手卡片列表
func (s *kuaishouCardService) GetList(ctx context.Context, req *dto.KuaishouCardListRequest) (*dto.KuaishouCardListResponse, error) {
	filter := repository.CardListFilter{Page: req.Page, PageSize: req.PageSize, Keyword: req.Keyword, IsActive: req.IsActive}
	cards, total, err := s.repo.GetList(ctx, filter)
	if err != nil {
		return nil, err
	}

	var responseList []dto.KuaishouCardResponse
	for _, card := range cards {
		// 获取短链信息
		shortCode := ""
		if card.ShortLinkID != nil && *card.ShortLinkID != 0 {
			shortLink, err := s.shortLinkService.GetByID(ctx, *card.ShortLinkID)
			if err == nil {
				shortCode = shortLink.ShortCode
			}
		}
		responseList = append(responseList, *s.toResponseWithShortLink(ctx, &card, shortCode))
	}

	return &dto.KuaishouCardListResponse{
		List:  responseList,
		Total: total,
	}, nil
}

// LikeCard 点赞卡片
func (s *kuaishouCardService) LikeCard(ctx context.Context, id uint) error {
	// 增加点赞数
	err := s.repo.IncrementLikeCount(ctx, id)
	if err != nil {
		return err
	}

	// 记录活动
	ip, _ := ctx.Value("ip").(string)
	userAgent, _ := ctx.Value("user_agent").(string)
	activity := &model.KuaishouCardActivity{
		CardID:       id,
		ActivityType: "like",
		IPAddress:    ip,
		UserAgent:    userAgent,
	}
	return s.repo.CreateActivity(ctx, activity)
}

// ViewCard 浏览卡片
func (s *kuaishouCardService) ViewCard(ctx context.Context, id uint) error {
	// 增加浏览数
	_, err := s.repo.IncrementViewCount(ctx, id)
	if err != nil {
		return err
	}

	// 记录活动
	ip, _ := ctx.Value("ip").(string)
	userAgent, _ := ctx.Value("user_agent").(string)
	activity := &model.KuaishouCardActivity{
		CardID:       id,
		ActivityType: "view",
		IPAddress:    ip,
		UserAgent:    userAgent,
	}
	return s.repo.CreateActivity(ctx, activity)
}

// ShareCard 分享卡片
func (s *kuaishouCardService) ShareCard(ctx context.Context, id uint, platform string) (*dto.KuaishouCardResponse, error) {
	// 获取卡片信息
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 增加分享数
	err = s.repo.IncrementShareCount(ctx, id)
	if err != nil {
		return nil, err
	}

	// 记录活动
	ip, _ := ctx.Value("ip").(string)
	userAgent, _ := ctx.Value("user_agent").(string)
	activity := &model.KuaishouCardActivity{
		CardID:       id,
		ActivityType: "share",
		IPAddress:    ip,
		UserAgent:    userAgent,
	}
	_ = s.repo.CreateActivity(ctx, activity)

	return s.toResponse(ctx, card), nil
}

// GenerateHTMLPage 生成快手卡片HTML页面
func (s *kuaishouCardService) GenerateHTMLPage(ctx context.Context, id uint) (string, error) {
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

	html, err := s.templateService.GenerateKuaishouCardPage(data)
	if err != nil {
		return "", err
	}

	return html, nil
}

// GenerateCardChatPage 生成快手卡片聊天页（统一模板，含联系客服按钮）
func (s *kuaishouCardService) GenerateCardChatPage(ctx context.Context, id uint, baseURL string) (string, error) {
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	return s.templateService.RenderCardChatPage("kuaishou", card.ID, card.Title, card.Description, card.ImageURL, card.Tags, baseURL)
}

// GenerateShortLink 生成短链接
func (s *kuaishouCardService) GenerateShortLink(ctx context.Context, card *model.KuaishouCard) error {
	// 检查是否已有短链
	if card.ShortLinkID != nil && *card.ShortLinkID != 0 {
		// 删除已有短链
		_ = s.shortLinkService.Delete(ctx, *card.ShortLinkID)
	}

	// 生成短码
	generateReq := &dto.GenerateShortCodeRequest{
		Length: 6,
	}
	generateResp, err := s.shortLinkService.GenerateShortCode(ctx, generateReq)
	if err != nil {
		return fmt.Errorf("生成短码失败: %v", err)
	}

	logger.Infof("生成的短码: %s", generateResp.ShortCode)

	// 获取域名池域名
	var domainID uint = 0 // 默认不绑定域名，避免依赖不存在的 domain_id=1
	if card.DomainPoolID != 0 {
		// 当用户指定了域名池 ID 时，使用该 ID 创建短链
		domainID = card.DomainPoolID
	}

	// 创建短链
	shortLinkReq := &dto.CreateShortLinkRequest{
		ShortCode:   generateResp.ShortCode,
		OriginalURL: fmt.Sprintf("/kuaishou/card/%d", card.ID), // 指向快手卡片页面
		Title:       card.Title,
		Description: card.Description,
		DomainID:    domainID, // 使用域名池ID作为域名ID
	}

	shortLinkResp, err := s.shortLinkService.Create(ctx, shortLinkReq)
	if err != nil {
		return fmt.Errorf("创建短链失败: %v", err)
	}

	logger.Infof("创建的短链ID: %d", shortLinkResp.ID)

	// 更新卡片的短链ID
	card.ShortLinkID = &shortLinkResp.ID
	_, err = s.repo.Update(ctx, card)
	if err != nil {
		return fmt.Errorf("更新卡片短链ID失败: %v", err)
	}

	logger.Infof("更新卡片短链ID成功: %d", *card.ShortLinkID)

	// 确保更新后的数据被正确保存，刷新数据库连接
	// 这是为了解决可能的缓存问题
	updatedCard, err := s.repo.GetByID(ctx, card.ID)
	if err != nil {
		return fmt.Errorf("获取更新后的卡片失败: %v", err)
	}

	// 验证短链ID是否正确更新
	if updatedCard.ShortLinkID == nil || *updatedCard.ShortLinkID != shortLinkResp.ID {
		return fmt.Errorf("短链ID更新失败")
	}

	logger.Infof("验证更新后的卡片短链ID: %d", *updatedCard.ShortLinkID)

	return nil
}

// toResponse 将模型转换为响应DTO
func (s *kuaishouCardService) toResponse(ctx context.Context, card *model.KuaishouCard) *dto.KuaishouCardResponse {
	// 获取短链信息
	shortCode := ""
	shortLinkURL := ""
	if card.ShortLinkID != nil && *card.ShortLinkID != 0 {
		shortLink, err := s.shortLinkService.GetByID(ctx, *card.ShortLinkID)
		if err == nil {
			shortCode = shortLink.ShortCode
			shortLinkURL = "/s/" + shortCode
		}
	}

	return &dto.KuaishouCardResponse{
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
		LikeCount:    card.LikeCount,
		ShareCount:   card.ShareCount,
		IsActive:     card.IsActive,
		CreatedAt:    card.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    card.UpdatedAt.Format(time.RFC3339),
	}
}

// toResponseWithShortLink 将模型转换为响应DTO，包含短链信息
func (s *kuaishouCardService) toResponseWithShortLink(ctx context.Context, card *model.KuaishouCard, shortCode string) *dto.KuaishouCardResponse {
	shortLinkURL := ""
	if shortCode != "" {
		shortLinkURL = "/s/" + shortCode
	}

	return &dto.KuaishouCardResponse{
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
		LikeCount:    card.LikeCount,
		ShareCount:   card.ShareCount,
		IsActive:     card.IsActive,
		CreatedAt:    card.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    card.UpdatedAt.Format(time.RFC3339),
	}
}
