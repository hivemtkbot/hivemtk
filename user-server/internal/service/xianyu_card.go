package service

import (
	"context"
	"fmt"
	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/reach/card/template"
	"marketing/internal/repository"

	"gorm.io/gorm"
)

// XianyuCardService 咸鱼卡片服务接口
type XianyuCardService interface {
	Create(ctx context.Context, req *dto.XianyuCardCreateRequest) (*dto.XianyuCardResponse, error)
	Update(ctx context.Context, req *dto.XianyuCardUpdateRequest) (*dto.XianyuCardResponse, error)
	Delete(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*dto.XianyuCardResponse, error)
	GetByIDWithRefresh(ctx context.Context, id uint) (*dto.XianyuCardResponse, error)
	GetCardModelByID(ctx context.Context, id uint) (*model.XianyuCard, error)
	GetList(ctx context.Context, req *dto.XianyuCardListRequest) (*dto.XianyuCardListResponse, error)
	ShareCard(ctx context.Context, id uint, platform string) error
	GenerateHTMLPage(ctx context.Context, id uint) (string, error)
	GenerateCardChatPage(ctx context.Context, id uint, baseURL string) (string, error)
	GenerateShortLink(ctx context.Context, card *model.XianyuCard) error
}

// xianyuCardService 咸鱼卡片服务实现
type xianyuCardService struct {
	repo              repository.XianyuCardRepository
	statsService      XianyuCardStatsService
	shortLinkService  ShortLinkService
	domainPoolService DomainPoolService
	templateService   *template.TemplateService
}

// NewXianyuCardService 创建咸鱼卡片服务
func NewXianyuCardService(db any) XianyuCardService {
	// 类型断言将interface{}转换为*gorm.DB
	gormDB := db.(*gorm.DB)
	return &xianyuCardService{
		repo:              repository.NewXianyuCardRepository(gormDB),
		statsService:      NewXianyuCardStatsService(gormDB),
		shortLinkService:  NewShortLinkService(gormDB),
		domainPoolService: NewDomainPoolService(gormDB),
		templateService:   template.NewTemplateService("internal/template"),
	}
}

// Create 创建咸鱼卡片
func (s *xianyuCardService) Create(ctx context.Context, req *dto.XianyuCardCreateRequest) (*dto.XianyuCardResponse, error) {
	// 创建卡片模型
	card := &model.XianyuCard{
		Title:        req.Title,
		Description:  req.Description,
		ImageURL:     req.ImageURL,
		RedirectURL:  req.RedirectURL,
		DomainPoolID: req.DomainPoolID,
		Tags:         req.Tags,
		IsActive:     req.IsActive,
	}

	// 保存到数据库
	if err := s.repo.Create(ctx, card); err != nil {
		return nil, fmt.Errorf("创建咸鱼卡片失败: %w", err)
	}

	// 生成短链（可选功能：失败时仅记日志，不影响主流程）
	if err := s.GenerateShortLink(ctx, card); err != nil {
		logger.Errorf("警告：生成短链失败：%v", err)
	}

	// 返回响应
	return s.convertToResponse(ctx, card), nil
}

// Update 更新咸鱼卡片
func (s *xianyuCardService) Update(ctx context.Context, req *dto.XianyuCardUpdateRequest) (*dto.XianyuCardResponse, error) {
	// 获取现有卡片
	card, err := s.repo.GetByID(ctx, req.ID)
	if err != nil {
		return nil, fmt.Errorf("获取咸鱼卡片失败: %w", err)
	}

	// 更新字段
	card.Title = req.Title
	card.Description = req.Description
	card.ImageURL = req.ImageURL
	card.RedirectURL = req.RedirectURL
	card.DomainPoolID = req.DomainPoolID
	card.Tags = req.Tags
	card.LikeCount = req.LikeCount
	card.ShareCount = req.ShareCount
	card.ViewCount = req.ViewCount
	card.IsActive = req.IsActive

	// 保存更新
	if err := s.repo.Update(ctx, card); err != nil {
		return nil, fmt.Errorf("更新咸鱼卡片失败: %w", err)
	}

	// 返回响应
	return s.convertToResponse(ctx, card), nil
}

// Delete 删除咸鱼卡片
func (s *xianyuCardService) Delete(ctx context.Context, id uint) error {
	return s.repo.Delete(ctx, id)
}

// GetByID 根据ID获取咸鱼卡片
func (s *xianyuCardService) GetByID(ctx context.Context, id uint) (*dto.XianyuCardResponse, error) {
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("获取咸鱼卡片失败: %w", err)
	}
	return s.convertToResponse(ctx, card), nil
}

// GetByIDWithRefresh 根据ID获取咸鱼卡片（带刷新）
func (s *xianyuCardService) GetByIDWithRefresh(ctx context.Context, id uint) (*dto.XianyuCardResponse, error) {
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("获取咸鱼卡片失败: %w", err)
	}

	// 刷新短链（可选功能：失败时仅记日志，不影响主流程）
	if err := s.GenerateShortLink(ctx, card); err != nil {
		logger.Errorf("警告：刷新短链失败：%v", err)
	}

	return s.convertToResponse(ctx, card), nil
}

// GetCardModelByID 根据ID获取咸鱼卡片模型
func (s *xianyuCardService) GetCardModelByID(ctx context.Context, id uint) (*model.XianyuCard, error) {
	return s.repo.GetByID(ctx, id)
}

// GetList 获取咸鱼卡片列表
func (s *xianyuCardService) GetList(ctx context.Context, req *dto.XianyuCardListRequest) (*dto.XianyuCardListResponse, error) {
	// 获取列表
	filter := repository.CardListFilter{Page: req.Page, PageSize: req.PageSize, Keyword: req.Keyword, IsActive: req.IsActive}
	cards, total, err := s.repo.GetList(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("获取咸鱼卡片列表失败: %w", err)
	}

	// 转换响应
	list := make([]dto.XianyuCardResponse, len(cards))
	for i, card := range cards {
		list[i] = *s.convertToResponse(ctx, &card)
	}

	// 计算总页数
	totalPage := int(total) / req.PageSize
	if int(total)%req.PageSize > 0 {
		totalPage++
	}

	return &dto.XianyuCardListResponse{
		List:      list,
		Total:     total,
		Page:      req.Page,
		PageSize:  req.PageSize,
		TotalPage: totalPage,
	}, nil
}

// ShareCard 分享咸鱼卡片
func (s *xianyuCardService) ShareCard(ctx context.Context, id uint, platform string) error {
	// 获取卡片
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("获取咸鱼卡片失败: %w", err)
	}

	// 根据平台增加分享数
	switch platform {
	case "wechat", "weixin":
		card.ShareCount++
	case "weibo":
		card.ShareCount++
	case "qq":
		card.ShareCount++
	default:
		card.ShareCount++
	}

	// 更新分享数
	return s.repo.Update(ctx, card)
}

// GenerateHTMLPage 生成HTML页面
func (s *xianyuCardService) GenerateHTMLPage(ctx context.Context, id uint) (string, error) {
	// 获取卡片
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return "", fmt.Errorf("获取咸鱼卡片失败: %w", err)
	}

	// 生成HTML内容
	htmlContent, err := s.templateService.RenderXianyuCard(card)
	if err != nil {
		return "", fmt.Errorf("渲染咸鱼卡片模板失败: %w", err)
	}

	return htmlContent, nil
}

// GenerateCardChatPage 生成咸鱼卡片聊天页（统一模板，含联系客服按钮）
func (s *xianyuCardService) GenerateCardChatPage(ctx context.Context, id uint, baseURL string) (string, error) {
	card, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return "", fmt.Errorf("获取咸鱼卡片失败: %w", err)
	}
	return s.templateService.RenderCardChatPage("xianyu", card.ID, card.Title, card.Description, card.ImageURL, card.Tags, baseURL)
}

// GenerateShortLink 生成短链
func (s *xianyuCardService) GenerateShortLink(ctx context.Context, card *model.XianyuCard) error {
	// 如果已经有短链，先删除旧的
	if card.ShortLinkID > 0 {
		if err := s.shortLinkService.Delete(context.Background(), card.ShortLinkID); err != nil {
			return fmt.Errorf("删除旧短链失败: %w", err)
		}
	}

	// 生成短码
	generateReq := &dto.GenerateShortCodeRequest{
		Length: 6,
	}
	generateResp, err := s.shortLinkService.GenerateShortCode(context.Background(), generateReq)
	if err != nil {
		return fmt.Errorf("生成短码失败：%w", err)
	}

	// 获取域名池域名
	var domainID uint = 0 // 默认不绑定域名，避免依赖不存在的 domain_id=1
	if card.DomainPoolID != 0 {
		domainID = card.DomainPoolID
	}

	// 创建短链
	shortLink, err := s.shortLinkService.Create(ctx, &dto.CreateShortLinkRequest{
		ShortCode:   generateResp.ShortCode,
		OriginalURL: fmt.Sprintf("/xianyu/card/%d", card.ID), // 指向咸鱼卡片页面
		Title:       card.Title,
		Description: card.Description,
		DomainID:    domainID,
	})
	if err != nil {
		return fmt.Errorf("创建短链失败: %w", err)
	}

	// 更新卡片的短链ID
	card.ShortLinkID = shortLink.ID
	return s.repo.Update(ctx, card)
}

// convertToResponse 转换模型到响应
func (s *xianyuCardService) convertToResponse(ctx context.Context, card *model.XianyuCard) *dto.XianyuCardResponse {
	// 获取短链信息
	shortLinkURL := ""
	shortCode := ""
	if card.ShortLinkID > 0 {
		shortLink, err := s.shortLinkService.GetByID(ctx, card.ShortLinkID)
		if err == nil && shortLink != nil {
			// 生成完整的短链URL
			shortLinkURL = "/s/" + shortLink.ShortCode
			shortCode = shortLink.ShortCode
		}
	}

	return &dto.XianyuCardResponse{
		ID:           card.ID,
		Title:        card.Title,
		Description:  card.Description,
		ImageURL:     card.ImageURL,
		RedirectURL:  card.RedirectURL,
		DomainPoolID: &card.DomainPoolID,
		ShortLinkURL: shortLinkURL,
		ShortCode:    shortCode,
		Tags:         card.Tags,
		LikeCount:    card.LikeCount,
		ShareCount:   card.ShareCount,
		ViewCount:    card.ViewCount,
		IsActive:     card.IsActive,
		CreatedAt:    card.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:    card.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}
