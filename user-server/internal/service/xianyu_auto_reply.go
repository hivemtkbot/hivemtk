package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"marketing/internal/aiagent/agent/browser"
	"marketing/internal/model"
	"marketing/internal/pkg/utils"
	"marketing/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

type XianyuAutoReplyService struct {
	db *gorm.DB
}

func NewXianyuAutoReplyService(db *gorm.DB) *XianyuAutoReplyService {
	return &XianyuAutoReplyService{db: db}
}

func (s *XianyuAutoReplyService) GetDB(ctx context.Context) *gorm.DB {
	return s.db
}

func (s *XianyuAutoReplyService) TestMatching(ctx context.Context, platform, message string, userID uint) (*model.AutoReplyRule, error) {
	var rules []model.AutoReplyRule
	err := s.db.Where(ctx, "platform = ? AND user_id = ? AND is_active = ?", platform, userID, true).Find(&rules).Error
	if err != nil {
		return nil, err
	}

	for _, rule := range rules {
		keywords := strings.Split(rule.Keywords, ",")
		for _, keyword := range keywords {
			if strings.Contains(message, strings.TrimSpace(keyword)) {
				return &rule, nil
			}
		}
	}
	return nil, nil
}

func (s *XianyuAutoReplyService) ListAccounts(ctx context.Context, userID uint) ([]model.AutoReplyAccount, error) {
	var items []model.AutoReplyAccount
	err := s.db.Where(ctx, "platform = ? AND user_id = ?", "xianyu", userID).Find(&items).Error
	return items, err
}

func (s *XianyuAutoReplyService) UpsertAccount(ctx context.Context, a *model.AutoReplyAccount) error {
	var existing model.AutoReplyAccount
	err := s.db.Where(ctx, "user_id = ? AND platform = ? AND username = ?", a.UserID, "xianyu", a.Username).First(&existing).Error
	if err == nil {
		// 加密存储Cookie(避免在 model 中保留业务方法)
		encrypted, encErr := utils.Encrypt(a.Cookie, utils.GetCookieEncryptionKey())
		if encErr != nil {
			return encErr
		}
		existing.Cookie = encrypted
		existing.IsActive = a.IsActive
		existing.LoginAt = a.LoginAt
		return s.db.Save(ctx, &existing).Error
	}
	// 加密存储Cookie(避免在 model 中保留业务方法)
	encrypted, encErr := utils.Encrypt(a.Cookie, utils.GetCookieEncryptionKey())
	if encErr != nil {
		return encErr
	}
	a.Cookie = encrypted
	return s.db.Create(ctx, a).Error
}

// SaveCookies 保存闲鱼账号 Cookie
//
// R8 修复：新增 userID 参数并校验账号所有权，防止 IDOR。
func (s *XianyuAutoReplyService) SaveCookies(ctx context.Context, id uint, cookie string, userID uint) error {
	// 获取现有的账号记录
	var account model.AutoReplyAccount
	if err := s.db.First(ctx, &account, id).Error; err != nil {
		return err
	}

	// 所有权校验：仅账号所有者可修改 Cookie
	if account.UserID != userID {
		return ErrAccountNotOwned
	}

	// 使用加密方法设置Cookie
	encrypted, encErr := utils.Encrypt(cookie, utils.GetCookieEncryptionKey())
	if encErr != nil {
		return encErr
	}
	account.Cookie = encrypted

	// 更新数据库中的加密Cookie
	return s.db.Model(ctx, &model.AutoReplyAccount{}).Where("id = ?", id).Update("cookie", account.Cookie).Error
}

func (s *XianyuAutoReplyService) GetRule(ctx context.Context, userID uint) (*model.AutoReplyRule, error) {
	var rule model.AutoReplyRule
	err := s.db.Where(ctx, "platform = ? AND user_id = ?", "xianyu", userID).Order("id DESC").First(&rule).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (s *XianyuAutoReplyService) SaveRule(ctx context.Context, rule *model.AutoReplyRule) error {
	var existing model.AutoReplyRule
	err := s.db.Where(ctx, "platform = ? AND user_id = ?", "xianyu", rule.UserID).First(&existing).Error
	if err == nil {
		existing.Keywords = rule.Keywords
		existing.ReplyContent = rule.ReplyContent
		existing.Frequency = rule.Frequency
		existing.DailyLimit = rule.DailyLimit
		existing.IsActive = rule.IsActive
		return s.db.Save(ctx, &existing).Error
	}
	return s.db.Create(ctx, rule).Error
}

func (s *XianyuAutoReplyService) ListRecentLogs(ctx context.Context, userID uint, page, pageSize int) ([]model.AutoReplyLog, int64, error) {
	var logs []model.AutoReplyLog
	cutoff := time.Now().Add(-72 * time.Hour)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	var total int64
	if err := s.db.Model(ctx, &model.AutoReplyLog{}).
		Where("platform = ? AND user_id = ? AND created_at >= ?", "xianyu", userID, cutoff).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := s.db.Where(ctx, "platform = ? AND user_id = ? AND created_at >= ?", "xianyu", userID, cutoff).
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&logs).Error
	return logs, total, err
}

func (s *XianyuAutoReplyService) AppendLog(ctx context.Context, userID, accountID, ruleID uint, platform, target, reply, status, errMsg string) error {
	item := &model.AutoReplyLog{UserID: userID, AccountID: accountID, RuleID: ruleID, Platform: platform, TargetContent: target, ReplyContent: reply, Status: status, ErrorMsg: errMsg, CreatedAt: time.Now()}
	return s.db.Create(ctx, item).Error
}

// StartLoginBrowser 启动本地浏览器打开咸鱼页面并在登录后提取 Cookie 保存
// 注意：需要服务器具备可用的 Chromium/Chrome 环境
func (s *XianyuAutoReplyService) StartLoginBrowser(ctx context.Context, userID uint, username string, accountID uint, headless bool) {
	go func() {
		logger.Infof("启动登录浏览器 - 平台: xianyu, 用户: %s, 无头模式: %v", username, headless)

		a, err := browser.NewAssistant(browser.Options{Headless: headless})
		if err != nil {
			logger.Errorf("创建浏览器助手失败: %v", err)
			return
		}
		defer a.Close()
		loginURL := browser.LoginURL(browser.Xianyu)
		logger.Infof("导航到登录页面: %s", loginURL)
		if err := a.Navigate(loginURL); err != nil {
			logger.Errorf("导航到登录页面失败: %v", err)
			return
		}
		logger.Infof("等待用户登录，超时时间: 5分钟")
		cookie, ok := a.WaitAuthCookieHeader(browser.Xianyu, 5*time.Minute)
		if ok {
			// 加密存储Cookie
			var account model.AutoReplyAccount
			if err := s.db.First(ctx, &account, accountID).Error; err == nil {
				encrypted, encErr := utils.Encrypt(cookie, utils.GetCookieEncryptionKey())
				if encErr == nil {
					account.Cookie = encrypted
					if err := s.db.Model(ctx, &model.AutoReplyAccount{}).Where("id = ?", accountID).Update("cookie", account.Cookie).Error; err != nil {
						logger.Errorf("保存Cookie失败: %v", err)
					} else {
						logger.Infof("设置Cookie成功")
					}
				} else {
					logger.Errorf("设置Cookie失败: %v", encErr)
				}
			}
		} else {
			logger.Errorf("平台 xianyu 用户 %s 登录超时或失败", username)
		}
	}()
}

func (s *XianyuAutoReplyService) DeleteAccount(ctx context.Context, id uint, userID uint) error {
	return s.db.Where(ctx, "id = ? AND user_id = ? AND platform = ?", id, userID, "xianyu").Delete(&model.AutoReplyAccount{}).Error
}

// MarkWSConnected 记录账号最近一次 WebSocket 连接成功时间
func (s *XianyuAutoReplyService) MarkWSConnected(ctx context.Context, accountID uint) error {
	return s.db.Model(ctx, &model.AutoReplyAccount{}).
		Where("id = ?", accountID).
		Update("last_ws_connected_at", time.Now()).Error
}

// StartWSBot 启动基于 WebSocket 的闲鱼自动回复机器人
// 优势：WebSocket 实时推送消息，无需轮询；只在发送回复时使用浏览器 DOM 操作
func (s *XianyuAutoReplyService) StartWSBot(ctx context.Context,
	bot *browser.AutoReplyBot,
	matcher browser.RuleMatcher,
	userID uint,
	rateLimiter *browser.RateLimiter,
	sliderSolver *browser.SliderSolver,
) error {
	if bot.IsRunning() {
		return errors.New("机器人已在运行中")
	}

	// 1. 设置平台（注入 Cookie + 导航到闲鱼IM）
	if err := bot.SetupPlatform(); err != nil {
		return fmt.Errorf("平台初始化失败: %w", err)
	}

	// 2. 从页面提取 WebSocket Token
	wsCfg, err := bot.ExtractXianyuWSToken()
	if err != nil {
		logger.Warnf("[闲鱼] 提取WS Token失败，降级为轮询模式: %v", err)
		// 降级为传统的 JS 轮询模式
		return bot.Start(matcher, userID)
	}

	// 3. 创建 WebSocket 客户端
	wsCfg.OnMessage = func(msg browser.XianyuChatMessage) {
		s.handleWSMessage(ctx, bot, msg, matcher, userID, rateLimiter, sliderSolver)
	}

	ws := browser.NewXianyuWebSocket(*wsCfg)

	// 4. 连接 WebSocket
	if err := ws.Connect(); err != nil {
		logger.Warnf("[闲鱼] WebSocket连接失败，降级为轮询模式: %v", err)
		return bot.Start(matcher, userID)
	}

	logger.Infof("[闲鱼] WebSocket 模式启动成功: %s", bot.GetAccount())
	return nil
}

// handleWSMessage 处理 WebSocket 收到的消息
func (s *XianyuAutoReplyService) handleWSMessage(ctx context.Context,
	bot *browser.AutoReplyBot,
	msg browser.XianyuChatMessage,
	matcher browser.RuleMatcher,
	userID uint,
	rateLimiter *browser.RateLimiter,
	sliderSolver *browser.SliderSolver,
) {
	logger.Infof("[闲鱼WS] 处理消息: sender=%s content=%s", msg.SenderName, msg.Content)

	// 1. 限流检查
	rateKey := fmt.Sprintf("xianyu_%s", bot.GetAccount())
	if allowed, reason := rateLimiter.Allow(rateKey); !allowed {
		logger.Infof("[闲鱼WS] %s，跳过回复", reason)
		return
	}

	// 2. 规则匹配
	rule, err := matcher.TestMatching(ctx, context.Background(), string(bot.GetPlatform()), msg.Content, userID)
	if err != nil || rule == nil {
		logger.Infof("[闲鱼WS] 无匹配规则，跳过")
		return
	}

	// 3. 滑块检测与处理
	if err := sliderSolver.Solve(bot); err != nil {
		logger.Errorf("[闲鱼WS] 滑块处理: %v", err)
	}

	// 4. 发送回复
	if err := bot.SendReply(msg.ChatID, rule.ReplyContent); err != nil {
		logger.Errorf("[闲鱼WS] 发送回复失败: %v", err)
		matcher.AppendLog(ctx, userID, bot.GetAccountID(), rule.ID, "xianyu", msg.Content, rule.ReplyContent, "failed", err.Error())
		return
	}

	// 5. 记录限流
	rateLimiter.Record(rateKey)

	// 6. 记录日志
	matcher.AppendLog(ctx, userID, bot.GetAccountID(), rule.ID, "xianyu", msg.Content, rule.ReplyContent, "sent", "")
	logger.Infof("[闲鱼WS] 回复发送成功: %s", rule.ReplyContent)
}
