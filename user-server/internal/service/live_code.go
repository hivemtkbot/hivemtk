package service

import (
	"errors"
	"fmt"
	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/repository"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LiveCodeService 活码服务接口
type LiveCodeService interface {
	Create(req *dto.CreateLiveCodeRequest) (*dto.LiveCodeResponse, error)
	Update(id string, req *dto.UpdateLiveCodeRequest) (*dto.LiveCodeResponse, error)
	Delete(id string) error
	// 轮询活码（每日200次，7天有效期）
	RotateLiveCodes() error
	// 记录点击统计
	RecordClick(id string, userAgent, referrer string) error

	// 根据ID获取活码
	GetByID(id string) (*dto.LiveCodeResponse, error)
	GetByShortLink(shortLink string) (*dto.LiveCodeResponse, error)
	GetList(page, pageSize int, name, status string) ([]*dto.LiveCodeResponse, int64, error)
	GetStats(id string) (*dto.LiveCodeStatsResponse, error)
	GenerateQRCode(id string, req *dto.GenerateQRCodeRequest) (*dto.LiveCodeQRResponse, error)
	GetQRCodes(id string) ([]*dto.LiveCodeQRResponse, error)
	GetQRStats(qrID string) (*dto.LiveCodeQRStatsResponse, error)
	Share(id string, req *dto.ShareLiveCodeRequest) (*dto.ShareLiveCodeResponse, error)
	DeleteQRCode(qrID string) error
	UpdateQRCode(qrID string, req *dto.UpdateLiveCodeQRRequest) error
}

// liveCodeService 活码服务实现
type liveCodeService struct {
	liveCodeRepo repository.LiveCodeRepository
	qrCodeRepo   repository.LiveCodeQRRepository
	qrStatRepo   repository.LiveCodeQRRepository
	domainRepo   repository.DomainPoolRepository
}

// NewLiveCodeService 创建活码服务实例
func NewLiveCodeService(db *gorm.DB) LiveCodeService {
	return &liveCodeService{
		liveCodeRepo: repository.NewLiveCodeRepository(db),
		qrCodeRepo:   repository.NewLiveCodeQRRepository(db),
		qrStatRepo:   repository.NewLiveCodeQRRepository(db),
		domainRepo:   repository.NewDomainPoolRepository(db),
	}
}

// Create 创建活码
func (s *liveCodeService) Create(req *dto.CreateLiveCodeRequest) (*dto.LiveCodeResponse, error) {
	// 检查短链是否已存在
	existingCode, _ := s.liveCodeRepo.GetByShortLink(req.ShortLink)
	if existingCode != nil {
		return nil, errors.New("短链已存在")
	}

	// 检查域名是否存在且可用
	shortDomain, err := s.domainRepo.GetByID(int(req.ShortDomainID))
	if err != nil {
		return nil, errors.New("短链域名不存在")
	}
	if shortDomain.Status != 1 {
		return nil, errors.New("短链域名不可用")
	}

	entryDomain, err := s.domainRepo.GetByID(int(req.EntryDomainID))
	if err != nil {
		return nil, errors.New("入口域名不存在")
	}
	if entryDomain.Status != 1 {
		return nil, errors.New("入口域名不可用")
	}

	landingDomain, err := s.domainRepo.GetByID(int(req.LandingDomainID))
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

	err = s.liveCodeRepo.Create(liveCode)
	if err != nil {
		return nil, err
	}

	return s.modelToResponse(liveCode), nil
}

// Update 更新活码
func (s *liveCodeService) Update(id string, req *dto.UpdateLiveCodeRequest) (*dto.LiveCodeResponse, error) {
	// 获取现有活码
	liveCode, err := s.liveCodeRepo.GetByID(id)
	if err != nil {
		return nil, errors.New("活码不存在")
	}

	// 如果更新了短链，检查新短链是否已存在
	if req.ShortLink != "" && req.ShortLink != liveCode.ShortLink {
		existingCode, _ := s.liveCodeRepo.GetByShortLink(req.ShortLink)
		if existingCode != nil && existingCode.ID != id {
			return nil, errors.New("短链已存在")
		}
		liveCode.ShortLink = req.ShortLink
	}

	// 如果更新了域名，检查域名是否存在且可用
	if req.ShortDomainID > 0 && req.ShortDomainID != liveCode.ShortDomainID {
		domain, err := s.domainRepo.GetByID(int(req.ShortDomainID))
		if err != nil {
			return nil, errors.New("短链域名不存在")
		}
		if domain.Status != 1 {
			return nil, errors.New("短链域名不可用")
		}
		liveCode.ShortDomainID = req.ShortDomainID
	}

	if req.EntryDomainID > 0 && req.EntryDomainID != liveCode.EntryDomainID {
		domain, err := s.domainRepo.GetByID(int(req.EntryDomainID))
		if err != nil {
			return nil, errors.New("入口域名不存在")
		}
		if domain.Status != 1 {
			return nil, errors.New("入口域名不可用")
		}
		liveCode.EntryDomainID = req.EntryDomainID
	}

	if req.LandingDomainID > 0 && req.LandingDomainID != liveCode.LandingDomainID {
		domain, err := s.domainRepo.GetByID(int(req.LandingDomainID))
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

	err = s.liveCodeRepo.Update(liveCode)
	if err != nil {
		return nil, err
	}

	return s.modelToResponse(liveCode), nil
}

// Delete 删除活码
func (s *liveCodeService) Delete(id string) error {
	_, err := s.liveCodeRepo.GetByID(id)
	if err != nil {
		return errors.New("活码不存在")
	}

	return s.liveCodeRepo.Delete(id)
}

// RotateLiveCodes 轮询活码（每日200次，7天有效期）
func (s *liveCodeService) RotateLiveCodes() error {
	// 获取所有可用的活码
	liveCodes, err := s.liveCodeRepo.GetAvailableLiveCodes()
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

		err := s.qrCodeRepo.Create(qrCode)
		if err != nil {
			continue // 跳过失败的二维码，继续处理下一个
		}
	}

	return nil
}

// RecordClick 记录点击统计
func (s *liveCodeService) RecordClick(qrID string, userAgent, referrer string) error {
	// 获取二维码信息
	qrCode, err := s.qrCodeRepo.GetByID(qrID)
	if err != nil {
		return err
	}

	// 更新二维码点击次数（父级 LiveCode.TotalClicks 累加由后续 liveCodeRepo.Update 完成）
	qrCode.RecordClick()
	err = s.qrCodeRepo.Update(qrCode)
	if err != nil {
		return err
	}

	// 获取活码信息
	liveCode, err := s.liveCodeRepo.GetByID(qrCode.LiveCodeID)
	if err != nil {
		return err
	}

	// 更新活码点击次数
	liveCode.TotalClicks++
	liveCode.DailyClicks++

	err = s.liveCodeRepo.Update(liveCode)
	if err != nil {
		return err
	}

	// 创建点击统计记录
	clickStat := &model.LiveCodeQRStat{
		QRCodeID: qrID,
		Date:     time.Now(),
	}

	return s.qrStatRepo.CreateStat(clickStat)
}

// GetByID 根据ID获取活码
func (s *liveCodeService) GetByID(id string) (*dto.LiveCodeResponse, error) {
	liveCode, err := s.liveCodeRepo.GetByID(id)
	if err != nil {
		return nil, errors.New("活码不存在")
	}

	return s.modelToResponse(liveCode), nil
}

// GetByShortLink 根据短链获取活码
func (s *liveCodeService) GetByShortLink(shortLink string) (*dto.LiveCodeResponse, error) {
	liveCode, err := s.liveCodeRepo.GetByShortLink(shortLink)
	if err != nil {
		return nil, errors.New("活码不存在")
	}

	return s.modelToResponse(liveCode), nil
}

// GetList 获取活码列表
func (s *liveCodeService) GetList(page, pageSize int, name, status string) ([]*dto.LiveCodeResponse, int64, error) {
	liveCodes, total, err := s.liveCodeRepo.GetList(page, pageSize, name, status)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*dto.LiveCodeResponse, len(liveCodes))
	for i, liveCode := range liveCodes {
		responses[i] = s.modelToResponse(liveCode)
	}

	return responses, total, nil
}

// GetStats 获取活码统计
func (s *liveCodeService) GetStats(id string) (*dto.LiveCodeStatsResponse, error) {
	liveCode, err := s.liveCodeRepo.GetByID(id)
	if err != nil {
		return nil, errors.New("活码不存在")
	}

	// 获取二维码统计
	qrCodes, err := s.qrCodeRepo.GetByLiveCodeID(id)
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
		// 注意：新模型中没有TotalShown和TotalClicks字段，暂时使用默认值0
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
func (s *liveCodeService) GenerateQRCode(id string, req *dto.GenerateQRCodeRequest) (*dto.LiveCodeQRResponse, error) {
	// 检查活码是否存在
	_, err := s.liveCodeRepo.GetByID(id)
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

	err = s.qrCodeRepo.Create(qrCode)
	if err != nil {
		return nil, err
	}

	return s.qrModelToResponse(qrCode), nil
}

// GetQRCodes 获取活码二维码列表
func (s *liveCodeService) GetQRCodes(id string) ([]*dto.LiveCodeQRResponse, error) {
	// 检查活码是否存在
	_, err := s.liveCodeRepo.GetByID(id)
	if err != nil {
		return nil, errors.New("活码不存在")
	}

	qrCodes, err := s.qrCodeRepo.GetByLiveCodeID(id)
	if err != nil {
		return nil, err
	}

	responses := make([]*dto.LiveCodeQRResponse, len(qrCodes))
	for i, qrCode := range qrCodes {
		responses[i] = s.qrModelToResponse(qrCode)
	}

	return responses, nil
}

// GetQRStats 获取活码二维码统计
func (s *liveCodeService) GetQRStats(qrID string) (*dto.LiveCodeQRStatsResponse, error) {
	qrCode, err := s.qrCodeRepo.GetByID(qrID)
	if err != nil {
		return nil, errors.New("二维码不存在")
	}

	// 获取二维码统计数据
	stats, err := s.qrCodeRepo.GetStats(qrID)
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

	return &dto.LiveCodeQRStatsResponse{
		QRCodeID:            qrCode.ID,
		ExpireDays:          qrCode.ExpireDays,
		ViewCount:           0,                                           // 新模型中没有此字段，设为默认值
		ClickCount:          0,                                           // 新模型中没有此字段，设为默认值
		ExpireTime:          time.Now().AddDate(0, 0, qrCode.ExpireDays), // 使用ExpireDays计算过期时间
		Status:              qrCode.Status,
		IsExpired:           false, // 新模型中没有此方法，设为默认值
		IsDailyLimitReached: false, // 新模型中没有此方法，设为默认值
		AccessStats:         accessStats,
	}, nil
}

// Share 分享活码
func (s *liveCodeService) Share(id string, req *dto.ShareLiveCodeRequest) (*dto.ShareLiveCodeResponse, error) {
	liveCode, err := s.liveCodeRepo.GetByID(id)
	if err != nil {
		return nil, errors.New("活码不存在")
	}

	// 获取可用的二维码
	var availableQR *model.LiveCodeQR
	qrCodes, err := s.qrCodeRepo.GetByLiveCodeID(id)
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

	// 返回分享链接
	return &dto.ShareLiveCodeResponse{
		ShortLink:   liveCode.GetFullShortLink(),
		EntryLink:   liveCode.GetFullEntryLink(),
		LandingLink: liveCode.GetFullLandingLink(),
		QRImagePath: availableQR.ImageURL,
		QRCodeID:    availableQR.ID,
	}, nil
}

// DeleteQRCode 删除二维码
func (s *liveCodeService) DeleteQRCode(qrID string) error {
	// 获取二维码信息
	_, err := s.qrCodeRepo.GetByID(qrID)
	if err != nil {
		return errors.New("二维码不存在")
	}

	// 删除二维码
	return s.qrCodeRepo.Delete(qrID)
}

// UpdateQRCode 更新二维码
func (s *liveCodeService) UpdateQRCode(qrID string, req *dto.UpdateLiveCodeQRRequest) error {
	// 获取二维码信息
	qrCode, err := s.qrCodeRepo.GetByID(qrID)
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

	return s.qrCodeRepo.Update(qrCode)
}

// modelToResponse 将模型转换为响应
func (s *liveCodeService) modelToResponse(liveCode *model.LiveCode) *dto.LiveCodeResponse {
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
		FullShortLink:   liveCode.GetFullShortLink(),
		FullEntryLink:   liveCode.GetFullEntryLink(),
		FullLandingLink: liveCode.GetFullLandingLink(),
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
func (s *liveCodeService) qrModelToResponse(qrCode *model.LiveCodeQR) *dto.LiveCodeQRResponse {
	return &dto.LiveCodeQRResponse{
		ID:                  qrCode.ID,
		LiveCodeID:          qrCode.LiveCodeID,
		ImageURL:            qrCode.ImageURL,
		QRImageURL:          qrCode.ImageURL,
		ExpireDays:          qrCode.ExpireDays,
		ViewCount:           0, // 新模型中没有此字段，设为默认值
		ClickCount:          0, // 新模型中没有此字段，设为默认值
		Status:              qrCode.Status,
		ExpireTime:          time.Now().AddDate(0, 0, qrCode.ExpireDays), // 使用ExpireDays计算过期时间
		CreatedAt:           qrCode.CreatedAt,
		UpdatedAt:           qrCode.UpdatedAt,
		IsExpired:           false, // 新模型中没有此方法，设为默认值
		IsDailyLimitReached: false, // 新模型中没有此方法，设为默认值
	}
}

// calculateConversionRate 计算转化率
func calculateConversionRate(shown, clicks int) float64 {
	if shown == 0 {
		return 0
	}
	return float64(clicks) / float64(shown) * 100
}
