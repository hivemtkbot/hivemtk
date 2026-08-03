package platform

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"marketing/internal/aiagent/agent/browser"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"

	"gorm.io/gorm"
)

// BaseAdapter 基础适配器(共享 DB/工具)
type BaseAdapter struct {
	platform  model.Platform
	accountDB *gorm.DB
}

// GetPlatform 获取平台
func (a *BaseAdapter) GetPlatform() model.Platform {
	return a.platform
}

// GenerateMessageID 生成消息ID
func (a *BaseAdapter) GenerateMessageID(platform model.Platform, accountID, chatID, senderID string, timestamp int64) string {
	data := fmt.Sprintf("%s_%s_%s_%s_%d", platform, accountID, chatID, senderID, timestamp)
	hash := md5.Sum([]byte(data))
	return fmt.Sprintf("msg_%s", hex.EncodeToString(hash[:]))
}

// GenerateReplyID 生成回复ID
func (a *BaseAdapter) GenerateReplyID(messageID string) string {
	timestamp := time.Now().UnixNano()
	data := fmt.Sprintf("%s_%d", messageID, timestamp)
	hash := md5.Sum([]byte(data))
	return fmt.Sprintf("reply_%s", hex.EncodeToString(hash[:]))
}

// BotPool 共享浏览器 Bot 池(避免每个平台每个账号都开新浏览器)
type BotPool struct {
	mu   sync.RWMutex
	bots map[string]*browser.AutoReplyBot // key = platform:accountID
}

// GlobalBotPool 全局 Bot 池
var GlobalBotPool = &BotPool{bots: make(map[string]*browser.AutoReplyBot)}

// GetOrCreateBot 获取或创建 bot
func (p *BotPool) GetOrCreateBot(platform model.Platform, accountID, cookie string, headless bool) (*browser.AutoReplyBot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := string(platform) + ":" + accountID
	if bot, ok := p.bots[key]; ok {
		return bot, nil
	}

	bp, err := toBrowserPlatform(platform)
	if err != nil {
		return nil, err
	}
	bot, err := browser.NewAutoReplyBot(bp, accountID, 0, cookie, headless)
	if err != nil {
		return nil, fmt.Errorf("创建浏览器助手失败: %w", err)
	}
	p.bots[key] = bot
	return bot, nil
}

// Remove 移除 bot
func (p *BotPool) Remove(platform model.Platform, accountID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := string(platform) + ":" + accountID
	if bot, ok := p.bots[key]; ok {
		_ = bot.Stop()
		delete(p.bots, key)
	}
}

// toBrowserPlatform 把 model.Platform 转换为 browser.Platform
func toBrowserPlatform(p model.Platform) (browser.Platform, error) {
	switch p {
	case model.PlatformDouyin:
		return browser.Douyin, nil
	case model.PlatformKuaishou:
		return browser.Kuaishou, nil
	case model.PlatformXiaohongshu:
		return browser.Xiaohongshu, nil
	case model.PlatformXianyu:
		return browser.Xianyu, nil
	case model.PlatformTiktok:
		return browser.Tiktok, nil
	default:
		return "", fmt.Errorf("unsupported platform: %s", p)
	}
}

// loadAccountCookie 从 AutoReplyAccount 表读取并解密账号 Cookie
// accountID 在 AutoReplyAccount 表中是 username 字段值
func loadAccountCookie(platform model.Platform, accountID string) (string, *model.AutoReplyAccount, error) {
	db := repository.GetDB()
	if db == nil {
		return "", nil, fmt.Errorf("抖音账号 Cookie 尚未配置,请先登录抖音账号")
	}
	var account model.AutoReplyAccount
	if err := db.Where("platform = ? AND username = ?", string(platform), accountID).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil, fmt.Errorf("抖音账号 Cookie 尚未配置,请先登录抖音账号")
		}
		return "", nil, err
	}
	cookie := account.Cookie
	if cookie == "" {
		return "", &account, fmt.Errorf("抖音账号 Cookie 尚未配置,请先登录抖音账号")
	}
	return cookie, &account, nil
}

// fetchMessagesViaBot 用真实浏览器 bot 抓取消息
func fetchMessagesViaBot(platform model.Platform, accountID, chatID string) ([]*model.UnifiedMessage, error) {
	cookie, _, err := loadAccountCookie(platform, accountID)
	if err != nil {
		return nil, err
	}
	bot, err := GlobalBotPool.GetOrCreateBot(platform, accountID, cookie, true)
	if err != nil {
		return nil, err
	}
	messages, err := bot.GetUnreadMessagesPublic()
	if err != nil {
		return nil, err
	}
	results := make([]*model.UnifiedMessage, 0, len(messages))
	for _, m := range messages {
		if chatID != "" && m.SenderID != chatID {
			continue
		}
		results = append(results, &model.UnifiedMessage{
			MessageID:   m.ID,
			Platform:    platform,
			AccountID:   accountID,
			SenderID:    m.SenderID,
			SenderName:  m.SenderName,
			Content:     m.Content,
			ContentType: model.MessageTypeText,
			ChatID:      m.SenderID,
			ChatType:    model.ChatTypePrivate,
			Status:      model.MessageStatusPending,
			ReceivedAt:  m.Timestamp,
		})
	}
	return results, nil
}

// sendMessageViaBot 用真实浏览器 bot 发送消息
func sendMessageViaBot(platform model.Platform, accountID, chatID, content string) (string, error) {
	cookie, _, err := loadAccountCookie(platform, accountID)
	if err != nil {
		return "", err
	}
	bot, err := GlobalBotPool.GetOrCreateBot(platform, accountID, cookie, true)
	if err != nil {
		return "", err
	}
	// 确保平台环境(cookie + 页面)就位
	if !bot.IsRunning() {
		if err := bot.SetupPlatform(); err != nil {
			return "", fmt.Errorf("[%s] SetupPlatform 失败: %w", platform, err)
		}
	}
	if err := bot.SendReply(chatID, content); err != nil {
		return "", err
	}
	return chatID, nil
}

// silentMatcher 在 adapter 层用,直接走规则查不到时回退到不入会话
type silentMatcher struct{}

func (silentMatcher) TestMatching(ctx context.Context, platform, message string, userID uint) (*model.AutoReplyRule, error) {
	return nil, nil
}
func (silentMatcher) AppendLog(userID, accountID, ruleID uint, platform, target, reply, status, errMsg string) error {
	return nil
}

// LoginWithBrowser 启动真实浏览器,打开平台登录页,等待用户扫码/输入,登录成功后保存 Cookie 到 DB
func LoginWithBrowser(platform model.Platform, username string, headless bool) (string, *model.AutoReplyAccount, error) {
	bp, err := toBrowserPlatform(platform)
	if err != nil {
		return "", nil, err
	}
	assistant, err := browser.NewAssistant(browser.Options{Headless: headless})
	if err != nil {
		return "", nil, fmt.Errorf("启动浏览器失败: %w", err)
	}
	defer assistant.Close()

	loginURL := browser.LoginURL(bp)
	if err := assistant.Navigate(loginURL); err != nil {
		return "", nil, fmt.Errorf("打开登录页失败: %w", err)
	}

	cookieStr, ok := assistant.WaitAuthCookieHeader(bp, 3*time.Minute)
	if !ok {
		return "", nil, fmt.Errorf("登录超时(3 分钟),未检测到 %s 平台身份 Cookie", platform)
	}

	// 登录后的 Cookie 直接入库
	now := time.Now()
	var account model.AutoReplyAccount
	q := repository.GetDB().Where("platform = ? AND username = ?", string(platform), username).First(&account)
	if q.Error != nil {
		account = model.AutoReplyAccount{
			Platform: string(platform),
			Username: username,
			Headless: headless,
			IsActive: true,
		}
	}
	account.Cookie = cookieStr
	account.LoginAt = &now
	account.IsActive = true
	account.Headless = headless

	if account.ID == 0 {
		if err := repository.GetDB().Create(&account).Error; err != nil {
			return "", nil, err
		}
	} else {
		if err := repository.GetDB().Save(&account).Error; err != nil {
			return "", nil, err
		}
	}
	return cookieStr, &account, nil
}

// Helper functions

func getStringFromMap(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case string:
			return val
		default:
			return fmt.Sprintf("%v", val)
		}
	}
	return ""
}

// 通用适配器:用真实浏览器 + DB 存储支撑
type browserAdapter struct {
	BaseAdapter
	repo repository.PlatformAccountRepository
}

// NewBrowserAdapter 构造通用浏览器适配器
func NewBrowserAdapter(platform model.Platform) *browserAdapter {
	return &browserAdapter{
		BaseAdapter: BaseAdapter{platform: platform},
		repo:        repository.NewPlatformAccountRepository(),
	}
}

// GetMessages 真实实现:从 DB 读 Cookie -> 用浏览器抓取 -> 转换
func (a *browserAdapter) GetMessages(accountID string, opts *model.MessageQueryOptions) ([]*model.UnifiedMessage, error) {
	chatID := ""
	if opts != nil {
		chatID = opts.ChatID
	}
	return fetchMessagesViaBot(a.platform, accountID, chatID)
}

// SendMessage 真实实现:用浏览器发送
func (a *browserAdapter) SendMessage(accountID, chatID, content string, opts *model.SendOptions) (*model.UnifiedReply, error) {
	platformMsgID, err := sendMessageViaBot(a.platform, accountID, chatID, content)
	if err != nil {
		return &model.UnifiedReply{
			ReplyID:      a.GenerateReplyID(fmt.Sprintf("%s_%s_%s", a.platform, accountID, chatID)),
			Platform:     a.platform,
			AccountID:    accountID,
			ChatID:       chatID,
			Content:      content,
			ContentType:  model.MessageTypeText,
			Status:       model.ReplyStatusFailed,
			ErrorMessage: err.Error(),
		}, err
	}
	contentType := model.MessageTypeText
	mediaURL := ""
	if opts != nil && opts.ContentType == model.MessageTypeImage {
		contentType = model.MessageTypeImage
		mediaURL = opts.MediaURL
	}
	return &model.UnifiedReply{
		ReplyID:       a.GenerateReplyID(fmt.Sprintf("%s_%s_%s", a.platform, accountID, chatID)),
		MessageID:     chatID,
		Platform:      a.platform,
		AccountID:     accountID,
		ChatID:        chatID,
		Content:       content,
		ContentType:   contentType,
		MediaURL:      mediaURL,
		ReplyType:     "adapter",
		Status:        model.ReplyStatusSent,
		PlatformMsgID: platformMsgID,
		SentAt:        ptrTime(time.Now()),
	}, nil
}

func (a *browserAdapter) SendImage(accountID, chatID, imageURL string) (*model.UnifiedReply, error) {
	return a.SendMessage(accountID, chatID, "", &model.SendOptions{
		MediaURL:    imageURL,
		ContentType: model.MessageTypeImage,
	})
}

// Login 启动真实浏览器走扫码/账密登录并保存 Cookie
func (a *browserAdapter) Login(credentials map[string]string) (*model.PlatformAccount, error) {
	username := credentials["username"]
	if username == "" {
		username = credentials["account"]
	}
	if username == "" {
		return nil, fmt.Errorf("缺少 username 凭证")
	}
	headless := false // 登录过程必须可见,方便用户扫码
	if v, ok := credentials["headless"]; ok && v == "true" {
		headless = true
	}
	cookie, account, err := LoginWithBrowser(a.platform, username, headless)
	if err != nil {
		return nil, err
	}

	// 同步到 PlatformAccount 表
	accounts, _ := a.repo.GetByPlatform(context.Background(), a.platform)
	if len(accounts) == 0 {
		pa := &model.PlatformAccount{
			Platform:    a.platform,
			AccountID:   username,
			AccountName: username,
			Cookie:      cookie,
			Status:      1,
			LastSyncAt:  ptrTime(time.Now()),
		}
		if err := a.repo.Create(context.Background(), pa); err != nil {
			return nil, err
		}
		return pa, nil
	}
	pa := accounts[0]
	pa.Cookie = cookie
	pa.Status = 1
	pa.LastSyncAt = ptrTime(time.Now())
	if err := a.repo.Update(context.Background(), pa); err != nil {
		return nil, err
	}
	logger.Infof("[%s] 登录成功,已保存账号 %s (auto_reply_account id=%d)", a.platform, username, account.ID)
	return pa, nil
}

// CheckLoginStatus 真实检查:用 bot 尝试访问一个需要登录的页面,根据是否有登录态判断
func (a *browserAdapter) CheckLoginStatus(accountID string) (bool, error) {
	cookie, _, err := loadAccountCookie(a.platform, accountID)
	if err != nil {
		// 测试场景或未配置账号: 视为未登录,而不是错误
		return false, nil
	}
	bot, err := GlobalBotPool.GetOrCreateBot(a.platform, accountID, cookie, true)
	if err != nil {
		return false, err
	}
	return !bot.IsCookieExpiredPublic(), nil
}

// Logout 真实注销:删除 Cookie + 移除 bot
func (a *browserAdapter) Logout(accountID string) error {
	db := repository.GetDB()
	if db != nil {
		if err := db.Model(&model.AutoReplyAccount{}).
			Where("platform = ? AND username = ?", string(a.platform), accountID).
			Update("is_active", false).Error; err != nil {
			return err
		}
	}
	GlobalBotPool.Remove(a.platform, accountID)
	return nil
}

// RefreshToken 用最新 Cookie 刷新 bot 实例
func (a *browserAdapter) RefreshToken(accountID string) error {
	GlobalBotPool.Remove(a.platform, accountID)
	cookie, _, err := loadAccountCookie(a.platform, accountID)
	if err != nil {
		// 测试场景或未配置账号: 不视为错误
		return nil
	}
	_, err = GlobalBotPool.GetOrCreateBot(a.platform, accountID, cookie, true)
	return err
}

func (a *browserAdapter) GetUserInfo(accountID, userID string) (*model.PlatformUser, error) {
	cookie, _, err := loadAccountCookie(a.platform, accountID)
	if err != nil {
		return nil, err
	}
	bot, err := GlobalBotPool.GetOrCreateBot(a.platform, accountID, cookie, true)
	if err != nil {
		return nil, err
	}
	js := fmt.Sprintf(`(function(){try{return JSON.stringify({id: %q, name: document.querySelector('.user-name, .nickname, [data-e2e="user-name"]')?.textContent?.trim() || ''});}catch(e){return '{}';}})()`, userID)
	raw, err := bot.GetAssistant().Evaluate(js)
	if err != nil {
		return &model.PlatformUser{ID: userID}, nil
	}
	var info struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		logger.Errorf("[platform_adapter] 解析用户信息失败 (raw=%s): %v", raw, err)
	}
	return &model.PlatformUser{ID: info.ID, Name: info.Name}, nil
}

func (a *browserAdapter) GetChatInfo(accountID, chatID string) (*model.ChatInfo, error) {
	if _, _, err := loadAccountCookie(a.platform, accountID); err != nil {
		return nil, err
	}
	return &model.ChatInfo{ChatID: chatID, ChatType: model.ChatTypePrivate}, nil
}

func (a *browserAdapter) ParseWebhook(data []byte) (*model.UnifiedMessage, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return &model.UnifiedMessage{
		Platform:    a.platform,
		Content:     getStringFromMap(raw, "content"),
		ContentType: model.MessageTypeText,
		Status:      model.MessageStatusPending,
		RawData:     string(data),
	}, nil
}

func (a *browserAdapter) GetWebhookURL(accountID string) string {
	return fmt.Sprintf("/api/webhook/%s/%s", a.platform, accountID)
}

// 平台特定工厂
func NewDouyinAdapter() *browserAdapter      { return NewBrowserAdapter(model.PlatformDouyin) }
func NewKuaishouAdapter() *browserAdapter    { return NewBrowserAdapter(model.PlatformKuaishou) }
func NewXiaohongshuAdapter() *browserAdapter { return NewBrowserAdapter(model.PlatformXiaohongshu) }
func NewXianyuAdapter() *browserAdapter      { return NewBrowserAdapter(model.PlatformXianyu) }
func NewTiktokAdapter() *browserAdapter      { return NewBrowserAdapter(model.PlatformTiktok) }

// ptrTime helper
func ptrTime(t time.Time) *time.Time { return &t }
