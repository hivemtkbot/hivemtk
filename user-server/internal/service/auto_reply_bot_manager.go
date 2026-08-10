package service

import (
	"context"
	"sync"
	"time"

	"hivemtk-user/internal/aiagent/agent/browser"
	knowledgesvc "hivemtk-user/internal/aiagent/knowledge/service"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
)

// ============================================================================
// 自动回复机器人编排 facade（B3 整改：controller 不直引 aiagent/browser）
//
// manager 单例、bot 生命周期编排、浏览器测试均下沉到 service 层；
// controller 只经本 facade 的纯 Go 类型签名交互。
// ============================================================================

// AutoReplyBotManager 浏览器自动回复机器人编排 facade（单例）
type AutoReplyBotManager struct {
	manager *browser.AutoReplyManager
	infra   *browser.AutoReplyInfra
}

var (
	botManagerOnce sync.Once
	botManagerInst *AutoReplyBotManager
)

// GetAutoReplyBotManager 返回单例编排 facade
func GetAutoReplyBotManager() *AutoReplyBotManager {
	botManagerOnce.Do(func() {
		// 默认所有平台都使用无头模式
		defaultHeadless := map[string]bool{
			"douyin":      true,
			"kuaishou":    true,
			"xiaohongshu": true,
			"xianyu":      true,
		}
		botManagerInst = &AutoReplyBotManager{
			manager: browser.NewAutoReplyManager(defaultHeadless),
			infra:   browser.GetAutoReplyInfra(),
		}
	})
	return botManagerInst
}

// SetHeadless 设置单平台无头模式
func (m *AutoReplyBotManager) SetHeadless(platform string, headless bool) {
	m.manager.SetHeadless(platform, headless)
}

// SyncHeadless 批量同步各平台无头设置
func (m *AutoReplyBotManager) SyncHeadless(settings map[string]bool) {
	for platform, headless := range settings {
		m.manager.SetHeadless(platform, headless)
	}
}

// StopBot 停止指定平台机器人
func (m *AutoReplyBotManager) StopBot(platform string) error {
	return m.manager.StopBot(browser.Platform(platform))
}

// BotStatus 机器人运行状态（Health 端点用）
type BotStatus struct {
	Running  bool
	Headless bool
}

// GetBotStatus 查询机器人运行状态（不存在视为未运行）
func (m *AutoReplyBotManager) GetBotStatus(platform string) BotStatus {
	bot, err := m.manager.GetBot(browser.Platform(platform))
	if err != nil || bot == nil {
		return BotStatus{}
	}
	return BotStatus{Running: bot.IsRunning(), Headless: bot.IsHeadless()}
}

// RateLimitStats 限流器统计（形状透传给 Health 端点）
func (m *AutoReplyBotManager) RateLimitStats(rateKey string) any {
	return m.infra.RateLimiter.Stats(rateKey)
}

// Infra 返回基础设施组件（仅供 service 包内 WS 编排使用）
func (m *AutoReplyBotManager) Infra() *browser.AutoReplyInfra {
	return m.infra
}

// PrepareBotWithReply 创建 bot 并装配去重与 RAG 回复 handler（不启动）
func (m *AutoReplyBotManager) PrepareBotWithReply(platform, username string, accountID uint, cookie string, ragStack *knowledgesvc.RAGStack, matcher browser.RuleMatcher) (*browser.AutoReplyBot, error) {
	bp := browser.Platform(platform)
	if err := m.manager.StartBot(bp, username, accountID, cookie); err != nil {
		return nil, err
	}
	bot, err := m.manager.GetBot(bp)
	if err != nil {
		return nil, err
	}
	bot.SetDedup(browser.NewInMemoryDedup(5 * time.Minute))
	bot.SetReplyHandler(browser.NewIntegrationReplyHandler(
		ragStack.Integration,
		ragStack.Customer,
		ragStack.Retrieval,
		matcher,
	))
	return bot, nil
}

// StartBotWithReply 通用启动编排：创建 bot → 装配 handler → 启动轮询
func (m *AutoReplyBotManager) StartBotWithReply(platform, username string, accountID uint, cookie string, ragStack *knowledgesvc.RAGStack, matcher browser.RuleMatcher, userID uint) error {
	bot, err := m.PrepareBotWithReply(platform, username, accountID, cookie, ragStack, matcher)
	if err != nil {
		return err
	}
	return bot.Start(matcher, userID)
}

// TestBrowserNavigation 浏览器导航连通性测试（URL 须已通过 SSRF 校验）
//
// 返回页面标题与截图是否成功。
func (m *AutoReplyBotManager) TestBrowserNavigation(url string, headless bool) (title string, screenshotOK bool, err error) {
	assistant, err := browser.NewAssistant(browser.Options{Headless: headless})
	if err != nil {
		return "", false, err
	}
	defer assistant.Close()
	if err := assistant.Navigate(url); err != nil {
		return "", false, err
	}
	title, err = assistant.Evaluate("document.title")
	if err != nil {
		return "", false, err
	}
	_, screenshotErr := assistant.Screenshot("body")
	return title, screenshotErr == nil, nil
}

// ============================================================================
// 全局 RAGStack 注入位（原 controller.SetRAGStack 上移）
// ============================================================================

var (
	ragStackMu  sync.RWMutex
	ragStackRef *knowledgesvc.RAGStack
)

// SetRAGStack 注入全局 RAGStack 实例（由装配层调用）
func SetRAGStack(s *knowledgesvc.RAGStack) {
	ragStackMu.Lock()
	defer ragStackMu.Unlock()
	ragStackRef = s
}

// GetRAGStack 返回全局 RAGStack 实例（未注入时为 nil）
func GetRAGStack() *knowledgesvc.RAGStack {
	ragStackMu.RLock()
	defer ragStackMu.RUnlock()
	return ragStackRef
}

// ============================================================================
// 各平台启动编排（从 controller 下沉）
// ============================================================================

// StartPlatformBot 抖音系通用启动编排：同步无头设置 → 启动 bot（handler 用独立规则服务）
func (s *AutoReplyService) StartPlatformBot(platform, username string, accountID, userID uint, cookie string, headlessSettings map[string]bool) error {
	m := GetAutoReplyBotManager()
	m.SyncHeadless(headlessSettings)
	replyService := NewAutoReplyServiceAuto()
	return m.StartBotWithReply(platform, username, accountID, cookie, GetRAGStack(), replyService, userID)
}

// StartXianyuBot 闲鱼启动编排（WS 模式失败降级轮询）；返回最终是否使用 WS 模式。
//
// 保持原行为：account.WsMode=false 时仅装配不启动轮询（历史语义）。
func (s *XianyuAutoReplyService) StartXianyuBot(ctx context.Context, account model.AutoReplyAccount, userID uint) (bool, error) {
	m := GetAutoReplyBotManager()
	m.SetHeadless("xianyu", account.Headless)
	bot, err := m.PrepareBotWithReply("xianyu", account.Username, account.ID, account.Cookie, GetRAGStack(), s)
	if err != nil {
		return false, err
	}
	if !account.WsMode {
		return false, nil
	}
	infra := m.Infra()
	if err := s.StartWSBot(ctx, bot, s, userID, infra.RateLimiter, infra.SliderSolver); err != nil {
		logger.Warnf("[闲鱼] WS 模式启动失败，降级为轮询: %v", err)
		if err := bot.Start(s, userID); err != nil {
			return false, err
		}
		return false, nil
	}
	// 记录 WS 连接成功时间
	if err := s.MarkWSConnected(ctx, account.ID); err != nil {
		logger.Errorf("[闲鱼] 记录 WS 连接时间失败: %v", err)
	}
	return true, nil
}

// StartXiaohongshuBot 小红书启动编排：装配 handler → 启动轮询
func (s *XiaohongshuAutoReplyService) StartXiaohongshuBot(ctx context.Context, account model.AutoReplyAccount, userID uint) error {
	m := GetAutoReplyBotManager()
	m.SetHeadless("xiaohongshu", account.Headless)
	bot, err := m.PrepareBotWithReply("xiaohongshu", account.Username, account.ID, account.Cookie, GetRAGStack(), s)
	if err != nil {
		return err
	}
	return bot.Start(s, userID)
}
