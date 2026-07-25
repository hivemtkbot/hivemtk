package service

import (
	"context"
	"strings"
	"time"

	"marketing/internal/aiagent/agent/browser"
	"marketing/internal/model"
	"marketing/internal/pkg/utils"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"

	"gorm.io/gorm"
)

type XiaohongshuAutoReplyService struct {
	db          *gorm.DB // 保留以维持 GetDB() 兼容
	accountRepo *repository.AutoReplyAccountRepository
	ruleRepo    *repository.AutoReplyRuleRepository
	logRepo     *repository.AutoReplyLogRepository
}

func NewXiaohongshuAutoReplyService(db *gorm.DB) *XiaohongshuAutoReplyService {
	return &XiaohongshuAutoReplyService{
		db:          db,
		accountRepo: repository.NewAutoReplyAccountRepository(db),
		ruleRepo:    repository.NewAutoReplyRuleRepositoryWithDB(db),
		logRepo:     repository.NewAutoReplyLogRepository(db),
	}
}

func (s *XiaohongshuAutoReplyService) GetDB(ctx context.Context) *gorm.DB { return s.db }

func (s *XiaohongshuAutoReplyService) TestMatching(ctx context.Context, platform, message string, userID uint) (*model.AutoReplyRule, error) {
	rules, err := s.ruleRepo.ListByPlatformAndUserActive(ctx, platform, userID)
	if err != nil {
		return nil, err
	}
	for _, rule := range rules {
		for _, kw := range strings.Split(rule.Keywords, ",") {
			if strings.Contains(message, strings.TrimSpace(kw)) {
				return &rule, nil
			}
		}
	}
	return nil, nil
}

func (s *XiaohongshuAutoReplyService) ListAccounts(ctx context.Context, userID uint) ([]model.AutoReplyAccount, error) {
	return s.accountRepo.ListByPlatformAndUser(ctx, "xiaohongshu", userID)
}

func (s *XiaohongshuAutoReplyService) UpsertAccount(ctx context.Context, a *model.AutoReplyAccount) error {
	existing, err := s.accountRepo.FindByUserAndPlatformAndUsername(ctx, a.UserID, "xiaohongshu", a.Username)
	if err == nil && existing != nil {
		// 加密存储Cookie(避免在 model 中保留业务方法)
		encrypted, encErr := utils.Encrypt(a.Cookie, utils.GetCookieEncryptionKey())
		if encErr != nil {
			return encErr
		}
		existing.Cookie = encrypted
		existing.IsActive = a.IsActive
		existing.LoginAt = a.LoginAt
		return s.accountRepo.Save(ctx, existing)
	}
	// 加密存储Cookie(避免在 model 中保留业务方法)
	encrypted, encErr := utils.Encrypt(a.Cookie, utils.GetCookieEncryptionKey())
	if encErr != nil {
		return encErr
	}
	a.Cookie = encrypted
	return s.accountRepo.Create(ctx, a)
}

// SaveCookies 保存小红书账号 Cookie
//
// R8 修复：新增 userID 参数并校验账号所有权，防止 IDOR。
func (s *XiaohongshuAutoReplyService) SaveCookies(ctx context.Context, id uint, cookie string, userID uint) error {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	// 所有权校验：仅账号所有者可修改 Cookie
	if account.UserID != userID {
		return ErrAccountNotOwned
	}
	// 加密存储Cookie(避免在 model 中保留业务方法)
	encrypted, encErr := utils.Encrypt(cookie, utils.GetCookieEncryptionKey())
	if encErr != nil {
		return encErr
	}
	account.Cookie = encrypted
	return s.accountRepo.UpdateCookieByID(ctx, id, account.Cookie)
}

func (s *XiaohongshuAutoReplyService) GetRule(ctx context.Context, userID uint) (*model.AutoReplyRule, error) {
	return s.ruleRepo.FindByPlatformAndUser(ctx, "xiaohongshu", userID)
}

func (s *XiaohongshuAutoReplyService) SaveRule(ctx context.Context, rule *model.AutoReplyRule) error {
	existing, err := s.ruleRepo.FindExistingByPlatformAndUser(ctx, "xiaohongshu", rule.UserID)
	if err == nil && existing != nil {
		existing.Keywords = rule.Keywords
		existing.ReplyContent = rule.ReplyContent
		existing.Frequency = rule.Frequency
		existing.DailyLimit = rule.DailyLimit
		existing.IsActive = rule.IsActive
		return s.ruleRepo.Save(ctx, existing)
	}
	return s.ruleRepo.Create(ctx, rule)
}

func (s *XiaohongshuAutoReplyService) ListRecentLogs(ctx context.Context, userID uint, page, pageSize int) ([]model.AutoReplyLog, int64, error) {
	cutoff := time.Now().Add(-72 * time.Hour)
	return s.logRepo.ListRecentByPlatformAndUser(ctx, "xiaohongshu", userID, page, pageSize, cutoff)
}

func (s *XiaohongshuAutoReplyService) AppendLog(userID, accountID, ruleID uint, platform, target, reply, status, errMsg string) error {
	item := &model.AutoReplyLog{
		UserID: userID, AccountID: accountID, RuleID: ruleID,
		Platform: platform, TargetContent: target, ReplyContent: reply,
		Status: status, ErrorMsg: errMsg, CreatedAt: time.Now(),
	}
	return s.logRepo.Create(context.Background(), item)
}

func (s *XiaohongshuAutoReplyService) StartLoginBrowser(ctx context.Context, userID uint, username string, accountID uint, headless bool) {
	go func() {
		ctx := context.Background()
		logger.Infof("启动登录浏览器 - 平台: xiaohongshu, 用户: %s", username)
		a, err := browser.NewAssistant(browser.Options{Headless: headless})
		if err != nil {
			logger.Errorf("创建浏览器助手失败: %v", err)
			return
		}
		defer a.Close()

		loginURL := browser.LoginURL(browser.Xiaohongshu)
		if err := a.Navigate(loginURL); err != nil {
			logger.Errorf("导航到登录页面失败: %v", err)
			return
		}

		cookie, ok := a.WaitAuthCookieHeader(browser.Xiaohongshu, 5*time.Minute)
		if ok {
			account, err := s.accountRepo.GetByID(ctx, accountID)
			if err == nil {
				encrypted, encErr := utils.Encrypt(cookie, utils.GetCookieEncryptionKey())
				if encErr == nil {
					account.Cookie = encrypted
					if err := s.accountRepo.UpdateCookieByID(ctx, accountID, account.Cookie); err != nil {
						logger.Errorf("保存Cookie失败: %v", err)
					} else {
						logger.Infof("小红书用户 %s 登录成功，Cookie 已保存", username)
					}
				} else {
					logger.Errorf("小红书用户 %s Cookie 加密失败: %v", username, encErr)
				}
			}
		}
	}()
}

func (s *XiaohongshuAutoReplyService) DeleteAccount(ctx context.Context, id uint, userID uint) error {
	return s.accountRepo.DeleteByIDUserAndPlatform(ctx, id, userID, "xiaohongshu")
}
