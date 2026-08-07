package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils"
	"marketing/internal/pkg/utils/config"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
	"math/big"
	"time"

	"gorm.io/gorm"
)

// ShortLinkService 短链服务接口
type ShortLinkService interface {
	Create(ctx context.Context, req *dto.CreateShortLinkRequest) (*dto.ShortLinkResponse, error)
	Update(ctx context.Context, req *dto.UpdateShortLinkRequest) (*dto.ShortLinkResponse, error)
	Delete(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*dto.ShortLinkResponse, error)
	GetByShortCode(ctx context.Context, shortCode string) (*dto.ShortLinkResponse, error)
	GetList(ctx context.Context, req *dto.ListShortLinkRequest) (*dto.ShortLinkListResponse, error)
	AccessShortLink(ctx context.Context, req *dto.AccessShortLinkRequest) (*dto.AccessShortLinkResponse, error)
	GenerateShortCode(ctx context.Context, req *dto.GenerateShortCodeRequest) (*dto.GenerateShortCodeResponse, error)
	GetStats(ctx context.Context, req *dto.ShortLinkStatsRequest) (*dto.ShortLinkStatsResponse, error)
	GetAllStats(ctx context.Context, req *dto.AllShortLinksStatsRequest) (*dto.AllShortLinksStatsResponse, error)
	ShareShortLink(ctx context.Context, req *dto.ShareShortLinkRequest) (*dto.ShareShortLinkResponse, error)
}

// IsShortLinkExpired 检查短链是否已过期（从 model 迁移而来的包级函数）
func IsShortLinkExpired(s *model.ShortLink) bool {
	if s == nil || s.ExpireTime == nil {
		return false
	}
	return time.Now().After(*s.ExpireTime)
}

// IsShortLinkActive 检查短链是否处于活跃状态（从 model 迁移而来的包级函数）
func IsShortLinkActive(s *model.ShortLink) bool {
	return s != nil && s.Status == 1 && !IsShortLinkExpired(s)
}

// shortLinkService 短链服务实现
type shortLinkService struct {
	shortLinkRepo repository.ShortLinkRepository
	domainRepo    repository.DomainPoolRepository
	accessRepo    repository.ShortLinkAccessRepository
}

// NewShortLinkService 创建短链服务实例
func NewShortLinkService(db *gorm.DB) ShortLinkService {
	return &shortLinkService{
		shortLinkRepo: repository.NewShortLinkRepository(db),
		domainRepo:    repository.NewDomainPoolRepository(db),
		accessRepo:    repository.NewShortLinkAccessRepository(db),
	}
}

// Create 创建短链
func (s *shortLinkService) Create(ctx context.Context, req *dto.CreateShortLinkRequest) (*dto.ShortLinkResponse, error) {
	// 检查短码是否已存在
	existingLink, _ := s.shortLinkRepo.GetByShortCode(context.Background(), req.ShortCode)
	if existingLink != nil {
		return nil, errors.New("短码已存在")
	}

	// 如果指定了域名ID，检查域名是否存在且可用
	if req.DomainID > 0 {
		domain, err := s.domainRepo.GetByID(ctx, int(req.DomainID))
		if err != nil {
			return nil, errors.New("域名不存在")
		}
		if domain.Status != 1 {
			return nil, errors.New("域名不可用")
		}
	}

	// 创建短链
	shortLink := &model.ShortLink{
		ShortCode:   req.ShortCode,
		OriginalURL: req.OriginalURL,
		Title:       req.Title,
		Description: req.Description,
		DomainID:    req.DomainID,
		Password:    req.Password,
		ExpireTime:  req.ExpireTime,
		Status:      1,
	}

	err := s.shortLinkRepo.Create(context.Background(), shortLink)
	if err != nil {
		return nil, err
	}

	return s.modelToResponse(ctx, shortLink), nil
}

// Update 更新短链
func (s *shortLinkService) Update(ctx context.Context, req *dto.UpdateShortLinkRequest) (*dto.ShortLinkResponse, error) {
	// 获取现有短链
	shortLink, err := s.shortLinkRepo.GetByID(context.Background(), req.ID)
	if err != nil {
		return nil, errors.New("短链不存在")
	}

	// 如果更新了短码，检查新短码是否已存在
	if req.ShortCode != "" && req.ShortCode != shortLink.ShortCode {
		existingLink, _ := s.shortLinkRepo.GetByShortCode(context.Background(), req.ShortCode)
		if existingLink != nil && existingLink.ID != req.ID {
			return nil, errors.New("短码已存在")
		}
		shortLink.ShortCode = req.ShortCode
	}

	// 如果指定了域名ID，检查域名是否存在且可用
	if req.DomainID > 0 && req.DomainID != shortLink.DomainID {
		domain, err := s.domainRepo.GetByID(ctx, int(req.DomainID))
		if err != nil {
			return nil, errors.New("域名不存在")
		}
		if domain.Status != 1 {
			return nil, errors.New("域名不可用")
		}
		shortLink.DomainID = req.DomainID
	}

	// 更新其他字段
	shortLink.OriginalURL = req.OriginalURL
	shortLink.Title = req.Title
	shortLink.Description = req.Description
	shortLink.Password = req.Password
	shortLink.ExpireTime = req.ExpireTime
	shortLink.Status = req.Status

	err = s.shortLinkRepo.Update(context.Background(), shortLink)
	if err != nil {
		return nil, err
	}

	return s.modelToResponse(ctx, shortLink), nil
}

// Delete 删除短链
func (s *shortLinkService) Delete(ctx context.Context, id uint) error {
	_, err := s.shortLinkRepo.GetByID(context.Background(), id)
	if err != nil {
		return errors.New("短链不存在")
	}

	return s.shortLinkRepo.Delete(context.Background(), id)
}

// GetByID 根据ID获取短链
func (s *shortLinkService) GetByID(ctx context.Context, id uint) (*dto.ShortLinkResponse, error) {
	shortLink, err := s.shortLinkRepo.GetByID(context.Background(), id)
	if err != nil {
		return nil, errors.New("短链不存在")
	}

	return s.modelToResponse(ctx, shortLink), nil
}

// GetByShortCode 根据短码获取短链
func (s *shortLinkService) GetByShortCode(ctx context.Context, shortCode string) (*dto.ShortLinkResponse, error) {
	shortLink, err := s.shortLinkRepo.GetByShortCode(context.Background(), shortCode)
	if err != nil {
		return nil, errors.New("短链不存在")
	}

	return s.modelToResponse(ctx, shortLink), nil
}

// GetList 获取短链列表
func (s *shortLinkService) GetList(ctx context.Context, req *dto.ListShortLinkRequest) (*dto.ShortLinkListResponse, error) {
	// 设置默认分页参数
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	shortLinks, total, err := s.shortLinkRepo.GetList(context.Background(), req.Page, req.PageSize, req.ShortCode, req.OriginalURL, req.Status)
	if err != nil {
		return nil, err
	}

	var responses []dto.ShortLinkResponse
	for _, shortLink := range shortLinks {
		responses = append(responses, *s.modelToResponse(ctx, shortLink))
	}

	return &dto.ShortLinkListResponse{
		List:  responses,
		Total: total,
	}, nil
}

// AccessShortLink 访问短链
func (s *shortLinkService) AccessShortLink(ctx context.Context, req *dto.AccessShortLinkRequest) (*dto.AccessShortLinkResponse, error) {
	// 根据短码获取短链
	shortLink, err := s.shortLinkRepo.GetByShortCode(context.Background(), req.ShortCode)
	if err != nil {
		return nil, errors.New("短链不存在")
	}

	// 检查短链是否处于活跃状态
	if !IsShortLinkActive(shortLink) {
		return nil, errors.New("短链已过期或已禁用")
	}

	// 如果设置了密码，验证密码
	if shortLink.Password != "" && shortLink.Password != req.Password {
		return nil, errors.New("密码错误")
	}

	// 增加点击次数
	err = s.shortLinkRepo.IncreaseClickCount(context.Background(), shortLink.ID)
	if err != nil {
		// 即使增加点击次数失败，也不影响访问
		logger.Errorf("增加点击次数失败: %v", err)
	}

	// 记录访问信息
	accessRecord := &model.ShortLinkAccess{
		ShortLinkID: shortLink.ID,
		IP:          req.IP,
		UserAgent:   req.UserAgent,
		Referer:     req.Referer,
		DeviceType:  utils.ParseDeviceType(req.UserAgent),
		Browser:     utils.ParseBrowser(req.UserAgent),
		OS:          utils.ParseOS(req.UserAgent),
		Location:    utils.ParseLocation(req.IP),
		AccessTime:  time.Now(),
	}

	err = s.accessRepo.Create(context.Background(), accessRecord)
	if err != nil {
		// 即使记录访问信息失败，也不影响访问
		logger.Errorf("记录访问信息失败: %v", err)
	}

	return &dto.AccessShortLinkResponse{
		OriginalURL: shortLink.OriginalURL,
		Title:       shortLink.Title,
	}, nil
}

// GenerateShortCode 生成短码
func (s *shortLinkService) GenerateShortCode(ctx context.Context, req *dto.GenerateShortCodeRequest) (*dto.GenerateShortCodeResponse, error) {
	// 设置默认长度
	if req.Length <= 0 {
		req.Length = 6
	}

	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	maxAttempts := 100

	for i := 0; i < maxAttempts; i++ {
		shortCode := s.generateRandomString(ctx, req.Length, charset)

		// 检查短码是否已存在
		_, err := s.shortLinkRepo.GetByShortCode(context.Background(), shortCode)
		if err != nil {
			// 短码不存在，可以使用
			return &dto.GenerateShortCodeResponse{
				ShortCode: shortCode,
			}, nil
		}
	}

	return nil, errors.New("生成短码失败，请重试")
}

// generateRandomString 生成随机字符串
func (s *shortLinkService) generateRandomString(ctx context.Context, length int, charset string) string {
	result := make([]byte, length)
	charsetLength := big.NewInt(int64(len(charset)))

	for i := 0; i < length; i++ {
		randomIndex, _ := rand.Int(rand.Reader, charsetLength)
		result[i] = charset[randomIndex.Int64()]
	}

	return string(result)
}

// modelToResponse 将模型转换为响应
func (s *shortLinkService) modelToResponse(ctx context.Context, shortLink *model.ShortLink) *dto.ShortLinkResponse {
	statusStr := "正常"
	if shortLink.Status == 2 {
		statusStr = "禁用"
	} else if IsShortLinkExpired(shortLink) {
		statusStr = "已过期"
	}

	return &dto.ShortLinkResponse{
		ID:          shortLink.ID,
		ShortCode:   shortLink.ShortCode,
		OriginalURL: shortLink.OriginalURL,
		Title:       shortLink.Title,
		Description: shortLink.Description,
		DomainID:    shortLink.DomainID,
		ExpireTime:  shortLink.ExpireTime,
		ClickCount:  shortLink.ClickCount,
		Status:      shortLink.Status,
		StatusStr:   statusStr,
		CreatedAt:   shortLink.CreatedAt,
		UpdatedAt:   shortLink.UpdatedAt,
	}
}

// GetStats 获取短链统计
func (s *shortLinkService) GetStats(ctx context.Context, req *dto.ShortLinkStatsRequest) (*dto.ShortLinkStatsResponse, error) {
	// 获取短链信息
	shortLink, err := s.shortLinkRepo.GetByID(context.Background(), req.ID)
	if err != nil {
		return nil, errors.New("短链不存在")
	}

	// 解析日期
	var startDate, endDate time.Time
	if req.StartDate != "" {
		startDate, err = time.Parse("2006-01-02", req.StartDate)
		if err != nil {
			return nil, errors.New("开始日期格式错误，请使用YYYY-MM-DD格式")
		}
	}

	if req.EndDate != "" {
		endDate, err = time.Parse("2006-01-02", req.EndDate)
		if err != nil {
			return nil, errors.New("结束日期格式错误，请使用YYYY-MM-DD格式")
		}
		// 设置为当天的23:59:59
		endDate = endDate.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
	}

	// 获取总访问量
	var totalAccess int64
	if startDate.IsZero() && endDate.IsZero() {
		// 如果没有指定日期范围，获取总访问量
		_, totalAccessCount, err := s.accessRepo.GetByShortLinkID(context.Background(), req.ID, 1, 0)
		if err != nil {
			totalAccess = 0
		} else {
			totalAccess = totalAccessCount
		}
	} else {
		// 获取指定日期范围内的访问量
		dailyStats, err := s.accessRepo.GetDailyStatsByShortLinkID(context.Background(), req.ID, startDate, endDate)
		if err == nil {
			for _, stat := range dailyStats {
				if count, ok := stat["count"].(int64); ok {
					totalAccess += count
				}
			}
		}
	}

	// 获取今日访问量
	today := time.Now().Format("2006-01-02")
	todayStart, _ := time.Parse("2006-01-02", today)
	todayEnd := todayStart.Add(23*time.Hour + 59*time.Minute + 59*time.Second)

	todayStats, err := s.accessRepo.GetDailyStatsByShortLinkID(context.Background(), req.ID, todayStart, todayEnd)
	var todayAccess int64
	if err == nil && len(todayStats) > 0 {
		if count, ok := todayStats[0]["count"].(int64); ok {
			todayAccess = count
		}
	}

	// 获取设备类型统计
	deviceTypeStats, err := s.accessRepo.GetDeviceTypeStatsByShortLinkID(context.Background(), req.ID, startDate, endDate)
	var deviceStats []dto.DeviceTypeStats
	if err == nil {
		for _, stat := range deviceTypeStats {
			deviceType, _ := stat["device_type"].(string)
			count, _ := stat["count"].(int64)

			// 计算百分比
			var percentage float64
			if totalAccess > 0 {
				percentage = float64(count) / float64(totalAccess) * 100
			}

			deviceStats = append(deviceStats, dto.DeviceTypeStats{
				DeviceType: deviceType,
				Count:      count,
				Percentage: percentage,
			})
		}
	}

	// 获取每日访问统计
	dailyStats, err := s.accessRepo.GetDailyStatsByShortLinkID(context.Background(), req.ID, startDate, endDate)
	var dailyStatsResponse []dto.DailyStats
	if err == nil {
		for _, stat := range dailyStats {
			date, _ := stat["date"].(string)
			count, _ := stat["count"].(int64)

			dailyStatsResponse = append(dailyStatsResponse, dto.DailyStats{
				Date:  date,
				Count: count,
			})
		}
	}

	return &dto.ShortLinkStatsResponse{
		ShortLinkID:     shortLink.ID,
		ShortCode:       shortLink.ShortCode,
		OriginalURL:     shortLink.OriginalURL,
		Title:           shortLink.Title,
		TotalAccess:     totalAccess,
		TodayAccess:     todayAccess,
		DeviceTypeStats: deviceStats,
		DailyStats:      dailyStatsResponse,
	}, nil
}

// GetAllStats 获取所有短链统计
func (s *shortLinkService) GetAllStats(ctx context.Context, req *dto.AllShortLinksStatsRequest) (*dto.AllShortLinksStatsResponse, error) {
	// 解析日期
	var startDate, endDate time.Time
	var err error

	if req.StartDate != "" {
		startDate, err = time.Parse("2006-01-02", req.StartDate)
		if err != nil {
			return nil, errors.New("开始日期格式错误，请使用YYYY-MM-DD格式")
		}
	}

	if req.EndDate != "" {
		endDate, err = time.Parse("2006-01-02", req.EndDate)
		if err != nil {
			return nil, errors.New("结束日期格式错误，请使用YYYY-MM-DD格式")
		}
		// 设置为当天的23:59:59
		endDate = endDate.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
	}

	// 获取总访问量
	var totalAccess int64
	dailyStats, err := s.accessRepo.GetAllDailyStats(context.Background(), startDate, endDate)
	if err == nil {
		for _, stat := range dailyStats {
			if count, ok := stat["count"].(int64); ok {
				totalAccess += count
			}
		}
	}

	// 获取今日访问量
	today := time.Now().Format("2006-01-02")
	todayStart, _ := time.Parse("2006-01-02", today)
	todayEnd := todayStart.Add(23*time.Hour + 59*time.Minute + 59*time.Second)

	todayStats, err := s.accessRepo.GetAllDailyStats(context.Background(), todayStart, todayEnd)
	var todayAccess int64
	if err == nil && len(todayStats) > 0 {
		for _, stat := range todayStats {
			if count, ok := stat["count"].(int64); ok {
				todayAccess += count
			}
		}
	}

	// 获取设备类型统计
	deviceTypeStats, err := s.accessRepo.GetAllDeviceTypeStats(context.Background(), startDate, endDate)
	var deviceStats []dto.DeviceTypeStats
	if err == nil {
		for _, stat := range deviceTypeStats {
			deviceType, _ := stat["device_type"].(string)
			count, _ := stat["count"].(int64)

			// 计算百分比
			var percentage float64
			if totalAccess > 0 {
				percentage = float64(count) / float64(totalAccess) * 100
			}

			deviceStats = append(deviceStats, dto.DeviceTypeStats{
				DeviceType: deviceType,
				Count:      count,
				Percentage: percentage,
			})
		}
	}

	// 获取每日访问统计
	dailyStats, err = s.accessRepo.GetAllDailyStats(context.Background(), startDate, endDate)
	var dailyStatsResponse []dto.DailyStats
	if err == nil {
		for _, stat := range dailyStats {
			date, _ := stat["date"].(string)
			count, _ := stat["count"].(int64)

			dailyStatsResponse = append(dailyStatsResponse, dto.DailyStats{
				Date:  date,
				Count: count,
			})
		}
	}

	// 获取各短链基本统计
	shortLinksStats, err := s.accessRepo.GetAllShortLinksBasicStats(context.Background(), startDate, endDate)
	var shortLinksStatsResponse []dto.ShortLinkBasicStats
	if err == nil {
		for _, stat := range shortLinksStats {
			id, _ := stat["id"].(uint)
			shortCode, _ := stat["short_code"].(string)
			title, _ := stat["title"].(string)
			accessCount, _ := stat["access_count"].(int64)

			shortLinksStatsResponse = append(shortLinksStatsResponse, dto.ShortLinkBasicStats{
				ID:          id,
				ShortCode:   shortCode,
				Title:       title,
				AccessCount: accessCount,
			})
		}
	}

	return &dto.AllShortLinksStatsResponse{
		TotalAccess:     totalAccess,
		TodayAccess:     todayAccess,
		DeviceTypeStats: deviceStats,
		DailyStats:      dailyStatsResponse,
		ShortLinkStats:  shortLinksStatsResponse,
	}, nil
}

// ShareShortLink 分享短链
func (s *shortLinkService) ShareShortLink(ctx context.Context, req *dto.ShareShortLinkRequest) (*dto.ShareShortLinkResponse, error) {
	// 获取短链信息
	shortLink, err := s.shortLinkRepo.GetByID(context.Background(), req.ID)
	if err != nil {
		return nil, errors.New("短链不存在")
	}

	// 构建短链URL（端口派生自 config.DefaultUserServerBaseURL）
	// 文档源：DEVELOPMENT.md §2.4 端口对照表 | 8204 | user-server
	shortURL := fmt.Sprintf("%s/s/%s", config.DefaultUserServerBaseURL, shortLink.ShortCode)

	// 生成二维码
	qrCode := utils.GenerateQRCode(shortURL)

	return &dto.ShareShortLinkResponse{
		ShortURL: shortURL,
		QRCode:   qrCode,
	}, nil
}
