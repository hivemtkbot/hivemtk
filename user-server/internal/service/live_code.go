package service

import (
	"errors"
	"fmt"
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
	"time"

	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LiveCodeService 活码服务接口
type LiveCodeService interface {
	Create(ctx context.Context, req *dto.CreateLiveCodeRequest) (*dto.LiveCodeResponse, error)
	Update(ctx context.Context, id string, req *dto.UpdateLiveCodeRequest) (*dto.LiveCodeResponse, error)
	Delete(ctx context.Context, id string) error
	// 轮询活码（每日200次，7天有效期）
	RotateLiveCodes(ctx context.Context) error
	// 记录点击统计
	RecordClick(ctx context.Context, id string, ip, userAgent, referrer string) error

	// 根据ID获取活码
	GetByID(ctx context.Context, id string) (*dto.LiveCodeResponse, error)
	GetByShortLink(ctx context.Context, shortLink string) (*dto.LiveCodeResponse, error)
	GetList(ctx context.Context, page, pageSize int, name, status string) ([]*dto.LiveCodeResponse, int64, error)
	GetStats(ctx context.Context, id string) (*dto.LiveCodeStatsResponse, error)
	GenerateQRCode(ctx context.Context, id string, req *dto.GenerateQRCodeRequest) (*dto.LiveCodeQRResponse, error)
	GetQRCodes(ctx context.Context, id string) ([]*dto.LiveCodeQRResponse, error)
	GetQRStats(ctx context.Context, qrID string) (*dto.LiveCodeQRStatsResponse, error)
	Share(ctx context.Context, id string, req *dto.ShareLiveCodeRequest) (*dto.ShareLiveCodeResponse, error)
	DeleteQRCode(ctx context.Context, qrID string) error
	UpdateQRCode(ctx context.Context, qrID string, req *dto.UpdateLiveCodeQRRequest) error
}

// liveCodeService 活码服务实现
type liveCodeService struct {
	liveCodeRepo repository.LiveCodeRepository
	qrCodeRepo   repository.LiveCodeQRRepository
	qrStatRepo   repository.LiveCodeQRRepository
	clickLogRepo repository.LiveCodeClickLogRepository
	domainRepo   repository.DomainPoolRepository
}

// NewLiveCodeService 创建活码服务实例
func NewLiveCodeService(db *gorm.DB) LiveCodeService {
	return &liveCodeService{
		liveCodeRepo: repository.NewLiveCodeRepository(db),
		qrCodeRepo:   repository.NewLiveCodeQRRepository(db),
		qrStatRepo:   repository.NewLiveCodeQRRepository(db),
		clickLogRepo: repository.NewLiveCodeClickLogRepository(db),
		domainRepo:   repository.NewDomainPoolRepository(db),
	}
}

// Create 创建活码
func (s *liveCodeService) Create(ctx context.Context, req *dto.CreateLiveCodeRequest) (*dto.LiveCodeResponse, error) {
	// 检查短链是否已存在
	existingCode, _ := s.liveCodeRepo.GetByShortLink(ctx, req.ShortLink)
	if existingCode != nil {
		return nil, errors.New("短链已存在")
	}

	// 检查域名是否存在且可用
	shortDomain, err := s.domainRepo.GetByID(ctx, int(req.ShortDomainID))
	if err != nil {
		return nil, errors.New("短链域名不存在")
	}
	if shortDomain.Status != 1 {
		return nil, errors.New("短链域名不可用")
	}

	entryDomain, err := s.domainRepo.GetByID(ctx, int(req.EntryDomainID))
	if err != nil {
		return nil, errors.New("入口域名不存在")
	}
	if entryDomain.Status != 1 {
		return nil, errors.New("入口域名不可用")
	}

	landingDomain, err := s.domainRepo.GetByID(ctx, int(req.LandingDomainID))
	if err != nil {
		return nil, errors.New("落地域名不存在")
	}
	if landingDomain.Status != 1 {
		return nil, errors.New("落地域名不可用")
	}

	// 创建活码
	liveCode := &model.LiveCode{
		ID:              uuid.New().String(),
		Name:            req.Name,
		ShortLink:       req.ShortLink,
		ShortDomainID:   req.ShortDomainID,
		EntryDomainID:   req.EntryDomainID,
		LandingDomainID: req.LandingDomainID,
		Status:          req.Status,
		TotalViews:      0,
		TodayViews:      0,
		ImageURL:        req.ImageURL,
		EntryURL:        req.EntryURL,
		LandingURL:      req.LandingURL,
	}

	err = s.liveCodeRepo.Create(ctx, liveCode)
	if err != nil {
		return nil, err
	}

	return s.modelToResponse(ctx, liveCode), nil
}

// Update 更新活码
func (s *liveCodeService) Update(ctx context.Context, id string, req *dto.UpdateLiveCodeRequest) (*dto.LiveCodeResponse, error) {
	// 获取现有活码
	liveCode, err := s.liveCodeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, errors.New("活码不存在")
	}

	// 如果更新了短链，检查新短链是否已存在
	if req.ShortLink != "" && req.ShortLink != liveCode.ShortLink {
		existingCode, _ := s.liveCodeRepo.GetByShortLink(ctx, req.ShortLink)
		if existingCode != nil && existingCode.ID != id {
			return nil, errors.New("短链已存在")
		}
		liveCode.ShortLink = req.ShortLink
	}

	// 如果更新了域名，检查域名是否存在且可用
	if req.ShortDomainID > 0 && req.ShortDomainID != liveCode.ShortDomainID {
		domain, err := s.domainRepo.GetByID(ctx, int(req.ShortDomainID))
		if err != nil {
			return nil, errors.New("短链域名不存在")
		}
		if domain.Status != 1 {
			return nil, errors.New("短链域名不可用")
		}
		liveCode.ShortDomainID = req.ShortDomainID
	}

	if req.EntryDomainID > 0 && req.EntryDomainID != liveCode.EntryDomainID {
		domain, err := s.domainRepo.GetByID(ctx, int(req.EntryDomainID))
		if err != nil {
			return nil, errors.New("入口域名不存在")
		}
		if domain.Status != 1 {
			return nil, errors.New("入口域名不可用")
		}
		liveCode.EntryDomainID = req.EntryDomainID
	}

	if req.LandingDomainID > 0 && req.LandingDomainID != liveCode.LandingDomainID {
		domain, err := s.domainRepo.GetByID(ctx, int(req.LandingDomainID))
		if err != nil {
			return nil, errors.New("落地域名不存在")
		}
		if domain.Status != 1 {
			return nil, errors.New("落地域名不可用")
		}
		liveCode.LandingDomainID = req.LandingDomainID
	}

	// 更新其他字段
	if req.Name != "" {
		liveCode.Name = req.Name
	}
	if req.Status != 0 {
		liveCode.Status = req.Status
	}
	if req.ImageURL != "" {
		liveCode.ImageURL = req.ImageURL
	}
	if req.EntryURL != "" {
		liveCode.EntryURL = req.EntryURL
	}
	if req.LandingURL != "" {
		liveCode.LandingURL = req.LandingURL
	}

	err = s.liveCodeRepo.Update(ctx, liveCode)
	if err != nil {
		return nil, err
	}

	return s.modelToResponse(ctx, liveCode), nil
}

// Delete 删除活码
func (s *liveCodeService) Delete(ctx context.Context, id string) error {
	_, err := s.liveCodeRepo.GetByID(ctx, id)
	if err != nil {
		return errors.New("活码不存在")
	}

	return s.liveCodeRepo.Delete(ctx, id)
}

// RotateLiveCodes 轮询活码（每日200次，7天有效期）
func (s *liveCodeService) RotateLiveCodes(ctx context.Context) error {
	// 获取所有可用的活码
	liveCodes, err := s.liveCodeRepo.GetAvailableLiveCodes(ctx)
	if err != nil {
		return err
	}

	// 为每个活码生成新的二维码
	for _, liveCode := range liveCodes {
		// 检查今日访问次数是否超过限制
		if liveCode.DailyClicks >= 200 {
			continue
		}

		// 生成新的二维码
		qrCode := &model.LiveCodeQR{
			LiveCodeID: liveCode.ID,
			Status:     1, // 活跃状态
		}

		err := s.qrCodeRepo.Create(ctx, qrCode)
		if err != nil {
			continue // 跳过失败的二维码，继续处理下一个
		}
	}

	return nil
}

// RecordClick 记录点击统计
func (s *liveCodeService) RecordClick(ctx context.Context, qrID string, ip, userAgent, referrer string) error {
	// 获取二维码信息
	qrCode, err := s.qrCodeRepo.GetByID(ctx, qrID)
	if err != nil {
		return err
	}

	// 校验父级活码存在，避免为已删除活码的孤儿二维码记录点击
	if _, err := s.liveCodeRepo.GetByID(ctx, qrCode.LiveCodeID); err != nil {
		return fmt.Errorf("活码 %s 不存在: %w", qrCode.LiveCodeID, err)
	}

	// 原子累加活码点击次数，避免并发读改写丢计数（lost-update）
	if err := s.liveCodeRepo.IncrementClicks(ctx, qrCode.LiveCodeID); err != nil {
		return err
	}

	// 累加二维码当天点击次数（按天聚合）
	if err := s.qrCodeRepo.IncrementClickStat(ctx, qrID); err != nil {
		return err
	}

	// 写入逐条点击审计日志（活码维度 + 二维码维度），用于安全审计与溯源
	if err := s.clickLogRepo.CreateLiveCodeClick(ctx, &model.LiveCodeClickLog{
		LiveCodeID: qrCode.LiveCodeID,
		QRCodeID:   qrID,
		IPAddress:  ip,
		UserAgent:  userAgent,
		Referrer:   referrer,
	}); err != nil {
		return err
	}
	if err := s.clickLogRepo.CreateQRCodeClick(ctx, &model.QRCodeClickLog{
		QRCodeID:   qrID,
		LiveCodeID: qrCode.LiveCodeID,
		IPAddress:  ip,
		UserAgent:  userAgent,
		Referrer:   referrer,
	}); err != nil {
		return err
	}

	return nil
}

// GetByID 根据ID获取活码
func (s *liveCodeService) GetByID(ctx context.Context, id string) (*dto.LiveCodeResponse, error) {
	liveCode, err := s.liveCodeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, errors.New("活码不存在")
	}

	return s.modelToResponse(ctx, liveCode), nil
}

// GetByShortLink 根据短链获取活码
func (s *liveCodeService) GetByShortLink(ctx context.Context, shortLink string) (*dto.LiveCodeResponse, error) {
	liveCode, err := s.liveCodeRepo.GetByShortLink(ctx, shortLink)
	if err != nil {
		return nil, errors.New("活码不存在")
	}

	return s.modelToResponse(ctx, liveCode), nil
}

// GetList 获取活码列表
func (s *liveCodeService) GetList(ctx context.Context, page, pageSize int, name, status string) ([]*dto.LiveCodeResponse, int64, error) {
	liveCodes, total, err := s.liveCodeRepo.GetList(ctx, page, pageSize, name, status)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*dto.LiveCodeResponse, len(liveCodes))
	for i, liveCode := range liveCodes {
		responses[i] = s.modelToResponse(ctx, liveCode)
	}

	return responses, total, nil
}

// GetStats 获取活码统计
func (s *liveCodeService) GetStats(ctx context.Context, id string) (*dto.LiveCodeStatsResponse, error) {
	liveCode, err := s.liveCodeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, errors.New("活码不存在")
	}

	// 获取二维码统计
	qrCodes, err := s.qrCodeRepo.GetByLiveCodeID(ctx, id)
	if err != nil {
		return nil, err
	}

	totalQRShown := 0
	totalQRClicks := 0
	activeQRCount := 0

	for _, qrCode := range qrCodes {
		if qrCode.Status == 1 {
			activeQRCount++
		}
	}

	// 从二维码按天聚合统计中汇总活码下所有二维码的展示/点击总数
	if shown, clicks, err := s.qrCodeRepo.SumLiveCodeStats(ctx, id); err == nil {
		totalQRShown = int(shown)
		totalQRClicks = int(clicks)
	}

	return &dto.LiveCodeStatsResponse{
		LiveCodeID:     id,
		TotalViews:     liveCode.TotalViews,
		TodayViews:     liveCode.TodayViews,
		TotalClicks:    liveCode.TotalClicks,
		TodayClicks:    liveCode.DailyClicks,
		TotalQRShown:   totalQRShown,
		TotalQRClicks:  totalQRClicks,
		ActiveQRCount:  activeQRCount,
		TotalQRCount:   len(qrCodes),
		ConversionRate: calculateConversionRate(totalQRShown, totalQRClicks),
	}, nil
}

// GenerateQRCode 生成活码二维码
func (s *liveCodeService) GenerateQRCode(ctx context.Context, id string, req *dto.GenerateQRCodeRequest) (*dto.LiveCodeQRResponse, error) {
	// 检查活码是否存在
	_, err := s.liveCodeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, errors.New("活码不存在")
	}

	// 生成二维码图片路径
	imagePath := fmt.Sprintf("/uploads/qrcodes/%s.png", uuid.New().String())

	// 创建二维码
	qrCode := &model.LiveCodeQR{
		ID:         uuid.New().String(),
		LiveCodeID: id,
		ImageURL:   imagePath,
		Status:     req.Status,
		ExpireDays: req.ExpireDays,
	}

	err = s.qrCodeRepo.Create(ctx, qrCode)
	if err != nil {
		return nil, err
	}

	return s.qrModelToResponse(ctx, qrCode), nil
}

// GetQRCodes 获取活码二维码列表
func (s *liveCodeService) GetQRCodes(ctx context.Context, id string) ([]*dto.LiveCodeQRResponse, error) {
	// 检查活码是否存在
	_, err := s.liveCodeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, errors.New("活码不存在")
	}

	qrCodes, err := s.qrCodeRepo.GetByLiveCodeID(ctx, id)
	if err != nil {
		return nil, err
	}

	responses := make([]*dto.LiveCodeQRResponse, len(qrCodes))
	for i, qrCode := range qrCodes {
		responses[i] = s.qrModelToResponse(ctx, qrCode)
	}

	return responses, nil
}

// GetQRStats 获取活码二维码统计
func (s *liveCodeService) GetQRStats(ctx context.Context, qrID string) (*dto.LiveCodeQRStatsResponse, error) {
	qrCode, err := s.qrCodeRepo.GetByID(ctx, qrID)
	if err != nil {
		return nil, errors.New("二维码不存在")
	}

	// 获取二维码统计数据
	stats, err := s.qrCodeRepo.GetStats(ctx, qrID)
	if err != nil {
		return nil, err
	}

	// 转换统计数据格式
	accessStats := make([]*dto.LiveCodeQRStatItem, len(stats))
	for i, stat := range stats {
		accessStats[i] = &dto.LiveCodeQRStatItem{
			ID:        fmt.Sprintf("%d", stat.ID), // 将int类型的ID转换为string
			QRCodeID:  stat.QRCodeID,
			Date:      stat.Date,
			CreatedAt: stat.CreatedAt,
		}
	}

	// 从二维码按天聚合统计中汇总历史展示/点击总数
	viewCount := 0
	clickCount := 0
	if shown, clicks, err := s.qrCodeRepo.SumStats(ctx, qrID); err == nil {
		viewCount = int(shown)
		clickCount = int(clicks)
	}

	return &dto.LiveCodeQRStatsResponse{
		QRCodeID:            qrCode.ID,
		ExpireDays:          qrCode.ExpireDays,
		ViewCount:           viewCount,
		ClickCount:          clickCount,
		ExpireTime:          time.Now().AddDate(0, 0, qrCode.ExpireDays), // 使用ExpireDays计算过期时间
		Status:              qrCode.Status,
		IsExpired:           false, // 新模型中没有此方法，设为默认值
		IsDailyLimitReached: false, // 新模型中没有此方法，设为默认值
		AccessStats:         accessStats,
	}, nil
}

// Share 分享活码
func (s *liveCodeService) Share(ctx context.Context, id string, req *dto.ShareLiveCodeRequest) (*dto.ShareLiveCodeResponse, error) {
	liveCode, err := s.liveCodeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, errors.New("活码不存在")
	}

	// 获取可用的二维码
	var availableQR *model.LiveCodeQR
	qrCodes, err := s.qrCodeRepo.GetByLiveCodeID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 轮询逻辑：找到状态为启用的二维码
	for _, qr := range qrCodes {
		if qr.Status == 1 {
			availableQR = qr
			break
		}
	}

	if availableQR == nil {
		return nil, errors.New("没有可用的二维码")
	}

	// 累加二维码当天展示次数（Share 即视为活码被展示一次）
	if err := s.qrCodeRepo.IncrementViewStat(ctx, availableQR.ID); err != nil {
		return nil, err
	}

	// 返回分享链接
	return &dto.ShareLiveCodeResponse{
		ShortLink:   liveCodeFullShortLink(liveCode),
		EntryLink:   liveCodeFullEntryLink(liveCode),
		LandingLink: liveCodeFullLandingLink(liveCode),
		QRImagePath: availableQR.ImageURL,
		QRCodeID:    availableQR.ID,
	}, nil
}

// DeleteQRCode 删除二维码
func (s *liveCodeService) DeleteQRCode(ctx context.Context, qrID string) error {
	// 获取二维码信息
	_, err := s.qrCodeRepo.GetByID(ctx, qrID)
	if err != nil {
		return errors.New("二维码不存在")
	}

	// 删除二维码
	return s.qrCodeRepo.Delete(ctx, qrID)
}

// UpdateQRCode 更新二维码
func (s *liveCodeService) UpdateQRCode(ctx context.Context, qrID string, req *dto.UpdateLiveCodeQRRequest) error {
	// 获取二维码信息
	qrCode, err := s.qrCodeRepo.GetByID(ctx, qrID)
	if err != nil {
		return errors.New("二维码不存在")
	}

	// 更新二维码信息
	if req.Status != nil {
		qrCode.Status = *req.Status
	}
	if req.ExpireDays != nil {
		qrCode.ExpireDays = *req.ExpireDays
	}

	return s.qrCodeRepo.Update(ctx, qrCode)
}

// modelToResponse 将模型转换为响应
func (s *liveCodeService) modelToResponse(ctx context.Context, liveCode *model.LiveCode) *dto.LiveCodeResponse {
	response := &dto.LiveCodeResponse{
		ID:              liveCode.ID,
		Name:            liveCode.Name,
		ShortLink:       liveCode.ShortLink,
		ShortDomainID:   liveCode.ShortDomainID,
		EntryDomainID:   liveCode.EntryDomainID,
		LandingDomainID: liveCode.LandingDomainID,
		Status:          liveCode.Status,
		TotalViews:      liveCode.TotalViews,
		TodayViews:      liveCode.TodayViews,
		CreatedAt:       liveCode.CreatedAt,
		UpdatedAt:       liveCode.UpdatedAt,
		FullShortLink:   liveCodeFullShortLink(liveCode),
		FullEntryLink:   liveCodeFullEntryLink(liveCode),
		FullLandingLink: liveCodeFullLandingLink(liveCode),
		ImageURL:        liveCode.ImageURL,
		EntryURL:        liveCode.EntryURL,
		LandingURL:      liveCode.LandingURL,
	}

	// 转换关联的域名数据
	if liveCode.ShortDomain != nil {
		response.ShortDomain = &dto.DomainPoolResponse{
			ID:        liveCode.ShortDomain.ID,
			Domain:    liveCode.ShortDomain.Domain,
			Port:      liveCode.ShortDomain.Port,
			Purpose:   liveCode.ShortDomain.Purpose,
			Status:    liveCode.ShortDomain.Status,
			CreatedAt: liveCode.ShortDomain.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt: liveCode.ShortDomain.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	if liveCode.EntryDomain != nil {
		response.EntryDomain = &dto.DomainPoolResponse{
			ID:        liveCode.EntryDomain.ID,
			Domain:    liveCode.EntryDomain.Domain,
			Port:      liveCode.EntryDomain.Port,
			Purpose:   liveCode.EntryDomain.Purpose,
			Status:    liveCode.EntryDomain.Status,
			CreatedAt: liveCode.EntryDomain.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt: liveCode.EntryDomain.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	if liveCode.LandingDomain != nil {
		response.LandingDomain = &dto.DomainPoolResponse{
			ID:        liveCode.LandingDomain.ID,
			Domain:    liveCode.LandingDomain.Domain,
			Port:      liveCode.LandingDomain.Port,
			Purpose:   liveCode.LandingDomain.Purpose,
			Status:    liveCode.LandingDomain.Status,
			CreatedAt: liveCode.LandingDomain.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt: liveCode.LandingDomain.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	return response
}

// qrModelToResponse 将二维码模型转换为响应
func (s *liveCodeService) qrModelToResponse(ctx context.Context, qrCode *model.LiveCodeQR) *dto.LiveCodeQRResponse {
	// 从二维码按天聚合统计中汇总历史展示/点击总数
	viewCount := 0
	clickCount := 0
	if shown, clicks, err := s.qrCodeRepo.SumStats(ctx, qrCode.ID); err == nil {
		viewCount = int(shown)
		clickCount = int(clicks)
	}

	return &dto.LiveCodeQRResponse{
		ID:                  qrCode.ID,
		LiveCodeID:          qrCode.LiveCodeID,
		ImageURL:            qrCode.ImageURL,
		QRImageURL:          qrCode.ImageURL,
		ExpireDays:          qrCode.ExpireDays,
		ViewCount:           viewCount,
		ClickCount:          clickCount,
		Status:              qrCode.Status,
		ExpireTime:          time.Now().AddDate(0, 0, qrCode.ExpireDays), // 使用ExpireDays计算过期时间
		CreatedAt:           qrCode.CreatedAt,
		UpdatedAt:           qrCode.UpdatedAt,
		IsExpired:           false, // 新模型中没有此方法，设为默认值
		IsDailyLimitReached: false, // 新模型中没有此方法，设为默认值
	}
}

// liveCodeFullShortLink 获取完整的短链（从 model.LiveCode 迁出，五层架构合规）
func liveCodeFullShortLink(l *model.LiveCode) string {
	if l.ShortDomain != nil {
		protocol := "http"
		if l.ShortDomain.Port == 443 {
			protocol = "https"
		}
		return protocol + "://" + l.ShortDomain.Domain + "/" + l.ShortLink
	}
	return ""
}

// liveCodeFullEntryLink 获取完整的入口链接（从 model.LiveCode 迁出）
func liveCodeFullEntryLink(l *model.LiveCode) string {
	if l.EntryDomain != nil {
		protocol := "http"
		if l.EntryDomain.Port == 443 {
			protocol = "https"
		}
		return protocol + "://" + l.EntryDomain.Domain + "/entry/" + l.ShortLink
	}
	return ""
}

// liveCodeFullLandingLink 获取完整的落地链接（从 model.LiveCode 迁出）
func liveCodeFullLandingLink(l *model.LiveCode) string {
	if l.LandingDomain != nil {
		protocol := "http"
		if l.LandingDomain.Port == 443 {
			protocol = "https"
		}
		return protocol + "://" + l.LandingDomain.Domain + "/landing/" + l.ShortLink
	}
	return ""
}

// calculateConversionRate 计算转化率
func calculateConversionRate(shown, clicks int) float64 {
	if shown == 0 {
		return 0
	}
	return float64(clicks) / float64(shown) * 100
}
