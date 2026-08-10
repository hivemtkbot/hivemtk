package service

import (
	"context"
	"fmt"
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
	"io"
	"net/http"
	"sync"
	"time"

	"gorm.io/gorm"
)

// DomainPoolService 域名池服务接口
type DomainPoolService interface {
	Create(ctx context.Context, domain string, port int, purpose string) (*model.DomainPool, error)
	Update(ctx context.Context, id int, domain string, port int, purpose string, status int) (*model.DomainPool, error)
	Delete(ctx context.Context, id int) error
	GetByID(ctx context.Context, id int) (*model.DomainPool, error)
	List(ctx context.Context, page, pageSize int, domain string, status int) ([]*model.DomainPool, int64, error)
	CheckDomain(ctx context.Context, id int) (bool, error)
	CheckAllDomains(ctx context.Context) ([]dto.DomainPoolCheckResponse, error)
	// Blacklist 域名黑名单管理(下沉到 Service 层,Controller 不再直访 Repository)
	AddBlacklist(ctx context.Context, domain, platform, reason, source string, ttlHours int) error
	RemoveBlacklist(ctx context.Context, domain string) error
	ListBlacklist(ctx context.Context, page, pageSize int) ([]*model.DomainBlacklist, int64, error)
}

// domainPoolService 域名池服务实现
type domainPoolService struct {
	domainPoolRepo repository.DomainPoolRepository
	blacklistRepo  *repository.DomainBlacklistRepository
	db             *gorm.DB
}

// NewDomainPoolService 创建域名池服务实例
func NewDomainPoolService(db *gorm.DB) DomainPoolService {
	return &domainPoolService{
		domainPoolRepo: repository.NewDomainPoolRepository(db),
		blacklistRepo:  repository.NewDomainBlacklistRepository(db),
		db:             db,
	}
}

// Create 创建域名池记录
func (s *domainPoolService) Create(ctx context.Context, domain string, port int, purpose string) (*model.DomainPool, error) {
	// 检查域名是否已存在
	existingDomain, _ := s.domainPoolRepo.GetByDomain(ctx, domain)
	if existingDomain != nil {
		return nil, fmt.Errorf("域名已存在")
	}

	// 设置默认端口
	if port <= 0 {
		port = 80
	}

	// 创建域名池记录
	domainPool := &model.DomainPool{
		Domain:    domain,
		Port:      port,
		Purpose:   purpose,
		Status:    1, // 默认状态为正常
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := s.domainPoolRepo.Create(ctx, domainPool)
	if err != nil {
		return nil, err
	}

	return domainPool, nil
}

// Update 更新域名池记录
func (s *domainPoolService) Update(ctx context.Context, id int, domain string, port int, purpose string, status int) (*model.DomainPool, error) {
	// 获取现有记录
	domainPool, err := s.domainPoolRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 检查域名是否已被其他记录使用
	if domain != domainPool.Domain {
		existingDomain, _ := s.domainPoolRepo.GetByDomain(ctx, domain)
		if existingDomain != nil && existingDomain.ID != id {
			return nil, fmt.Errorf("域名已被其他记录使用")
		}
	}

	// 设置默认端口
	if port <= 0 {
		port = 80
	}

	// 更新记录
	domainPool.Domain = domain
	domainPool.Port = port
	domainPool.Purpose = purpose
	domainPool.Status = status
	domainPool.UpdatedAt = time.Now()

	err = s.domainPoolRepo.Update(ctx, domainPool)
	if err != nil {
		return nil, err
	}

	return domainPool, nil
}

// Delete 删除域名池记录
func (s *domainPoolService) Delete(ctx context.Context, id int) error {
	return s.domainPoolRepo.Delete(context.Background(), id)
}

// GetByID 根据ID获取域名池记录
func (s *domainPoolService) GetByID(ctx context.Context, id int) (*model.DomainPool, error) {
	return s.domainPoolRepo.GetByID(context.Background(), id)
}

// List 获取域名池列表
func (s *domainPoolService) List(ctx context.Context, page, pageSize int, domain string, status int) ([]*model.DomainPool, int64, error) {
	return s.domainPoolRepo.List(context.Background(), page, pageSize, domain, status)
}

// CheckDomain 检查单个域名是否可访问
func (s *domainPoolService) CheckDomain(ctx context.Context, id int) (bool, error) {
	// 获取域名记录
	domainPool, err := s.domainPoolRepo.GetByID(ctx, id)
	if err != nil {
		return false, err
	}

	// 构建URL
	url := fmt.Sprintf("http://%s:%d", domainPool.Domain, domainPool.Port)

	// 发送HTTP请求检查域名是否可访问
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		// 更新状态为不可访问
		s.domainPoolRepo.UpdateStatus(ctx, id, 2)
		s.domainPoolRepo.UpdateLastCheck(ctx, id, time.Now())
		return false, nil
	}
	defer resp.Body.Close()

	// 更新状态为可访问
	s.domainPoolRepo.UpdateStatus(ctx, id, 1)
	s.domainPoolRepo.UpdateLastCheck(ctx, id, time.Now())

	return true, nil
}

// CheckAllDomains 并发检查所有域名是否可访问（默认并发 domainCheckConcurrency，避免串行逐个 HTTP 阻塞）
func (s *domainPoolService) CheckAllDomains(ctx context.Context) ([]dto.DomainPoolCheckResponse, error) {
	// 获取所有域名
	domainPools, _, err := s.domainPoolRepo.List(ctx, 1, 1000, "", 0)
	if err != nil {
		return nil, err
	}
	if len(domainPools) == 0 {
		return nil, nil
	}

	results := make([]dto.DomainPoolCheckResponse, len(domainPools))
	var wg sync.WaitGroup
	sem := make(chan struct{}, domainCheckConcurrency)
	for i, dp := range domainPools {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, dp *model.DomainPool) {
			defer wg.Done()
			defer func() { <-sem }()

			// 构建URL
			url := fmt.Sprintf("http://%s:%d", dp.Domain, dp.Port)

			client := &http.Client{Timeout: 5 * time.Second}

			status := 2 // 默认不可访问
			msg := "不可访问"

			resp, err := client.Get(url)
			if err != nil {
				msg = fmt.Sprintf("连接错误: %s", err.Error())
			} else {
				// 及时排空并关闭响应体，避免串行 defer 堆积导致连接泄漏
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode < 400 {
					status = 1
					msg = "可访问"
				} else {
					msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
				}
			}

			// 更新数据库中的状态和最后检查时间（gorm 连接池并发安全）
			s.domainPoolRepo.UpdateStatus(ctx, dp.ID, status)
			s.domainPoolRepo.UpdateLastCheck(ctx, dp.ID, time.Now())

			results[i] = dto.DomainPoolCheckResponse{
				ID:     dp.ID,
				Status: status,
				Msg:    msg,
			}
		}(i, dp)
	}
	wg.Wait()
	return results, nil
}

// domainCheckConcurrency CheckAllDomains 并发健康检查的最大并发数
const domainCheckConcurrency = 16

// ============== G 域 黑名单管理(下沉到 Service) ==============

// AddBlacklist 添加域名到黑名单
// ttlHours=0 表示永久,>0 表示 TTL(小时)
func (s *domainPoolService) AddBlacklist(ctx context.Context, domain, platform, reason, source string, ttlHours int) error {
	var expiresAt *time.Time
	if ttlHours > 0 {
		t := time.Now().Add(time.Duration(ttlHours) * time.Hour)
		expiresAt = &t
	}
	return s.blacklistRepo.Add(ctx, domain, platform, reason, source, expiresAt)
}

// RemoveBlacklist 从黑名单移除域名(软删除:置 active=false)
func (s *domainPoolService) RemoveBlacklist(ctx context.Context, domain string) error {
	return s.blacklistRepo.Remove(ctx, domain)
}

// ListBlacklist 查询黑名单分页列表
func (s *domainPoolService) ListBlacklist(ctx context.Context, page, pageSize int) ([]*model.DomainBlacklist, int64, error) {
	return s.blacklistRepo.List(ctx, page, pageSize)
}
