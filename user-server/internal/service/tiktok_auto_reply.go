package service

import (
	"errors"
	"strconv"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils"
	"marketing/internal/repository"

	"context"
	"gorm.io/gorm"
)

// TikTokAutoReplyService TikTok 自动回复服务(基于通用 AutoReplyService,锁定 platform=tiktok)
type TikTokAutoReplyService struct {
	db          *gorm.DB
	accountRepo *repository.AutoReplyAccountRepository
	reply       *AutoReplyService
	platform    string
}

const TikTokPlatform = "tiktok"

// NewTikTokAutoReplyService 创建 TikTok 自动回复服务
func NewTikTokAutoReplyService() *TikTokAutoReplyService {
	accountRepo := repository.NewAutoReplyAccountRepositoryAuto()
	replySvc := NewAutoReplyServiceAuto()
	return &TikTokAutoReplyService{
		db:          nil, // 兼容旧字段类型，不再用于直接 SQL；所有 SQL 通过 accountRepo 走仓储
		accountRepo: accountRepo,
		reply:       replySvc,
		platform:    TikTokPlatform,
	}
}

// ListAccounts 列出 TikTok 自动回复账号
func (s *TikTokAutoReplyService) ListAccounts(ctx context.Context, userID uint) ([]model.AutoReplyAccount, error) {
	accounts, err := s.reply.ListAccounts(ctx, s.platform, userID)
	if err != nil {
		return nil, err
	}
	if accounts == nil {
		accounts = []model.AutoReplyAccount{}
	}
	return accounts, nil
}

// UpsertAccountRequest 创建/更新账号请求
type UpsertAccountRequest struct {
	ID          uint   `json:"id"`
	Username    string `json:"username" binding:"required"`
	DisplayName string `json:"display_name"`
	IsActive    bool   `json:"is_active"`
	Cookie      string `json:"cookie"`
}

// UpsertAccount 创建或更新 TikTok 账号
func (s *TikTokAutoReplyService) UpsertAccount(ctx context.Context, userID uint, req *UpsertAccountRequest) (*model.AutoReplyAccount, error) {
	if userID == 0 {
		return nil, errors.New("user_id 不能为空")
	}
	if req.Username == "" {
		return nil, errors.New("username 不能为空")
	}
	account := &model.AutoReplyAccount{
		UserID:   userID,
		Platform: s.platform,
		Username: req.Username,
		IsActive: req.IsActive,
		Headless: true,
	}
	if req.Cookie != "" {
		encrypted, encErr := utils.Encrypt(req.Cookie, utils.GetCookieEncryptionKey())
		if encErr != nil {
			return nil, encErr
		}
		account.Cookie = encrypted
	}
	if err := s.reply.UpsertAccount(ctx, account); err != nil {
		return nil, err
	}
	return account, nil
}

// GetRule 获取 TikTok 自动回复规则
func (s *TikTokAutoReplyService) GetRule(ctx context.Context, userID uint) (*model.AutoReplyRule, error) {
	rule, err := s.reply.GetRule(ctx, s.platform, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &model.AutoReplyRule{
				UserID:       userID,
				Platform:     s.platform,
				Keywords:     "",
				ReplyContent: "",
				Frequency:    60,
				DailyLimit:   100,
				IsActive:     false,
			}, nil
		}
		return nil, err
	}
	return rule, nil
}

// SaveRuleRequest 保存规则请求
type SaveRuleRequest struct {
	Keywords     string  `json:"keywords"`
	ReplyContent string  `json:"reply_content"`
	Frequency    int     `json:"frequency"`
	DailyLimit   int     `json:"daily_limit"`
	StartTime    *string `json:"start_time"`
	EndTime      *string `json:"end_time"`
	IsActive     bool    `json:"is_active"`
}

// SaveRule 保存 TikTok 规则
func (s *TikTokAutoReplyService) SaveRule(ctx context.Context, userID uint, req *SaveRuleRequest) (*model.AutoReplyRule, error) {
	if userID == 0 {
		return nil, errors.New("user_id 不能为空")
	}
	rule := &model.AutoReplyRule{
		UserID:       userID,
		Platform:     s.platform,
		Keywords:     req.Keywords,
		ReplyContent: req.ReplyContent,
		Frequency:    req.Frequency,
		DailyLimit:   req.DailyLimit,
		StartTime:    req.StartTime,
		EndTime:      req.EndTime,
		IsActive:     req.IsActive,
	}
	if err := s.reply.SaveRule(ctx, rule); err != nil {
		return nil, err
	}
	return rule, nil
}

// SaveCookies 保存 TikTok 账号 Cookie
//
// R8 修复：透传 userID 给底层 AutoReplyService.SaveCookies 做所有权校验。
func (s *TikTokAutoReplyService) SaveCookies(ctx context.Context, id uint, cookie string, userID uint) error {
	if id == 0 {
		return errors.New("账号ID不能为空")
	}
	return s.reply.SaveCookies(ctx, id, cookie, userID)
}

// DeleteAccount 删除 TikTok 账号
func (s *TikTokAutoReplyService) DeleteAccount(ctx context.Context, id, userID uint) error {
	return s.reply.DeleteAccount(ctx, id, userID)
}

// ListLogs 列出 TikTok 自动回复日志
func (s *TikTokAutoReplyService) ListLogs(ctx context.Context, userID uint, page, pageSize int) ([]model.AutoReplyLog, int64, error) {
	return s.reply.ListRecentLogs(ctx, s.platform, userID, page, pageSize)
}

// Start 启动 TikTok 自动回复(在数据库写入运行状态)
func (s *TikTokAutoReplyService) Start(ctx context.Context, userID uint) error {
	if userID == 0 {
		return errors.New("user_id 不能为空")
	}
	// 通过 repository 更新账号激活状态
	return s.accountRepo.UpdateActiveByPlatformAndUser(ctx, s.platform, userID, true)
}

// Stop 停止 TikTok 自动回复
func (s *TikTokAutoReplyService) Stop(ctx context.Context, userID uint) error {
	if userID == 0 {
		return errors.New("user_id 不能为空")
	}
	// 通过 repository 更新账号激活状态
	return s.accountRepo.UpdateActiveByPlatformAndUser(ctx, s.platform, userID, false)
}

// Status 返回自动回复的运行状态
func (s *TikTokAutoReplyService) Status(ctx context.Context, userID uint) (map[string]any, error) {
	activeCount, err := s.accountRepo.CountActiveByPlatformAndUser(ctx, s.platform, userID)
	if err != nil {
		return nil, err
	}
	totalCount, err := s.accountRepo.CountByPlatformAndUser(ctx, s.platform, userID)
	if err != nil {
		return nil, err
	}

	rule, _ := s.GetRule(ctx, userID)
	replying := false
	if rule != nil && rule.IsActive {
		replying = true
	}
	return map[string]any{
		"running":      replying,
		"active_count": activeCount,
		"total_count":  totalCount,
		"checked_at":   time.Now().Format("2006-01-02 15:04:05"),
		"platform":     s.platform,
		"user_id":      strconv.FormatUint(uint64(userID), 10),
	}, nil
}
