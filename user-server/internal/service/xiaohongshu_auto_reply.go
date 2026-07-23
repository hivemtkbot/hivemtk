package service

import (
	"context"
	"strings"
	"time"

	"marketing/internal/aiagent/agent/browser"
	"marketing/internal/model"
	"marketing/internal/pkg/utils"
	"marketing/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

type XiaohongshuAutoReplyService struct {
	db *gorm.DB
}

func NewXiaohongshuAutoReplyService(db *gorm.DB) *XiaohongshuAutoReplyService {
	return &XiaohongshuAutoReplyService{db: db}
}

func (s *XiaohongshuAutoReplyService) GetDB(ctx context.Context) *gorm.DB { return s.db }

func (s *XiaohongshuAutoReplyService) TestMatching(ctx context.Context, platform, message string, userID uint) (*model.AutoReplyRule, error) {
	var rules []model.AutoReplyRule
	if err := s.db.WithContext(ctx).Where("platform = ? AND user_id = ? AND is_active = ?", platform, userID, true).Find(&rules).Error; err != nil {
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
	var items []model.AutoReplyAccount
	err := s.db.WithContext(ctx).Where("platform = ? AND user_id = ?", "xiaohongshu", userID).Find(&items).Error
	return items, err
}

func (s *XiaohongshuAutoReplyService) UpsertAccount(ctx context.Context, a *model.AutoReplyAccount) error {
	var existing model.AutoReplyAccount
	err := s.db.WithContext(ctx).Where("user_id = ? AND platform = ? AND username = ?", a.UserID, "xiaohongshu", a.Username).First(&existing).Error
	if err == nil {
		// 加密存储Cookie(避免在 model 中保留业务方法)
		encrypted, encErr := utils.Encrypt(a.Cookie, utils.GetCookieEncryptionKey())
		if encErr != nil {
			return encErr
		}
		existing.Cookie = encrypted
		existing.IsActive = a.IsActive
		existing.LoginAt = a.LoginAt
		return s.db.Save(&existing).Error
	}
	// 加密存储Cookie(避免在 model 中保留业务方法)
	encrypted, encErr := utils.Encrypt(a.Cookie, utils.GetCookieEncryptionKey())
	if encErr != nil {
		return encErr
	}
	a.Cookie = encrypted
	return s.db.Create(a).Error
}

// SaveCookies 保存小红书账号 Cookie
//
// R8 修复：新增 userID 参数并校验账号所有权，防止 IDOR。
func (s *XiaohongshuAutoReplyService) SaveCookies(ctx context.Context, id uint, cookie string, userID uint) error {
	var account model.AutoReplyAccount
	if err := s.db.WithContext(ctx).First(&account, id).Error; err != nil {
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
	return s.db.Model(&model.AutoReplyAccount{}).Where("id = ?", id).Update("cookie", account.Cookie).Error
}

func (s *XiaohongshuAutoReplyService) GetRule(ctx context.Context, userID uint) (*model.AutoReplyRule, error) {
	var rule model.AutoReplyRule
	err := s.db.WithContext(ctx).Where("platform = ? AND user_id = ?", "xiaohongshu", userID).Order("id DESC").First(&rule).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (s *XiaohongshuAutoReplyService) SaveRule(ctx context.Context, rule *model.AutoReplyRule) error {
	var existing model.AutoReplyRule
	err := s.db.WithContext(ctx).Where("platform = ? AND user_id = ?", "xiaohongshu", rule.UserID).First(&existing).Error
	if err == nil {
		existing.Keywords = rule.Keywords
		existing.ReplyContent = rule.ReplyContent
		existing.Frequency = rule.Frequency
		existing.DailyLimit = rule.DailyLimit
		existing.IsActive = rule.IsActive
		return s.db.Save(&existing).Error
	}
	return s.db.Create(rule).Error
}

func (s *XiaohongshuAutoReplyService) ListRecentLogs(ctx context.Context, userID uint, page, pageSize int) ([]model.AutoReplyLog, int64, error) {
	var logs []model.AutoReplyLog
	cutoff := time.Now().Add(-72 * time.Hour)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	var total int64
	s.db.Model(&model.AutoReplyLog{}).
		Where("platform = ? AND user_id = ? AND created_at >= ?", "xiaohongshu", userID, cutoff).
		Count(&total)
	err := s.db.WithContext(ctx).Where("platform = ? AND user_id = ? AND created_at >= ?", "xiaohongshu", userID, cutoff).
		Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&logs).Error
	return logs, total, err
}

func (s *XiaohongshuAutoReplyService) AppendLog(userID, accountID, ruleID uint, platform, target, reply, status, errMsg string) error {
	item := &model.AutoReplyLog{
		UserID: userID, AccountID: accountID, RuleID: ruleID,
		Platform: platform, TargetContent: target, ReplyContent: reply,
		Status: status, ErrorMsg: errMsg, CreatedAt: time.Now(),
	}
	return s.db.Create(item).Error
}

func (s *XiaohongshuAutoReplyService) StartLoginBrowser(ctx context.Context, userID uint, username string, accountID uint, headless bool) {
	go func() {
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
			var account model.AutoReplyAccount
			if err := s.db.WithContext(ctx).First(&account, accountID).Error; err == nil {
				encrypted, encErr := utils.Encrypt(cookie, utils.GetCookieEncryptionKey())
				if encErr == nil {
					account.Cookie = encrypted
					s.db.Model(&model.AutoReplyAccount{}).Where("id = ?", accountID).Update("cookie", account.Cookie)
					logger.Infof("小红书用户 %s 登录成功，Cookie 已保存", username)
				} else {
					logger.Errorf("小红书用户 %s Cookie 加密失败: %v", username, encErr)
				}
			}
		}
	}()
}

func (s *XiaohongshuAutoReplyService) DeleteAccount(ctx context.Context, id uint, userID uint) error {
	return s.db.WithContext(ctx).Where("id = ? AND user_id = ? AND platform = ?", id, userID, "xiaohongshu").Delete(&model.AutoReplyAccount{}).Error
}
