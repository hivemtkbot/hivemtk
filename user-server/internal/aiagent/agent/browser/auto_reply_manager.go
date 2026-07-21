package browser

import (
	"fmt"
	"marketing/internal/pkg/utils/logger"
	"sync"
)

// AutoReplyManager 自动回复管理器
type AutoReplyManager struct {
	bots     map[string]*AutoReplyBot // 平台 -> 机器人实例
	botsMux  sync.RWMutex
	headless map[string]bool // 各平台无头模式设置
}

// NewAutoReplyManager 创建自动回复管理器
func NewAutoReplyManager(headless map[string]bool) *AutoReplyManager {
	return &AutoReplyManager{
		bots:     make(map[string]*AutoReplyBot),
		headless: headless,
	}
}

// StartBot 启动指定平台的机器人
func (m *AutoReplyManager) StartBot(platform Platform, account string, accountID uint, cookies string) error {
	m.botsMux.Lock()
	defer m.botsMux.Unlock()

	platformKey := string(platform)

	// 检查是否已存在运行的机器人
	if bot, exists := m.bots[platformKey]; exists {
		if bot.IsRunning() {
			return fmt.Errorf("[%s] 机器人已在运行中", platform)
		}
		// 停止已存在的机器人
		bot.Stop()
		delete(m.bots, platformKey)
	}

	// 获取该平台的无头模式设置，默认使用无头模式
	headless, exists := m.headless[platformKey]
	if !exists {
		headless = true
	}

	// 创建新的机器人实例
	bot, err := NewAutoReplyBot(platform, account, accountID, cookies, headless)
	if err != nil {
		return fmt.Errorf("创建机器人失败: %v", err)
	}

	m.bots[platformKey] = bot

	// 启动机器人 - 这里我们还不能启动，因为缺少matcher参数
	// 机器人将在控制器层面启动，那里有完整的上下文
	logger.Infof("[%s] 机器人实例创建成功，等待启动: %s (无头模式: %v)", platform, account, headless)
	return nil
}

// StopBot 停止指定平台的机器人
func (m *AutoReplyManager) StopBot(platform Platform) error {
	m.botsMux.Lock()
	defer m.botsMux.Unlock()

	platformKey := string(platform)
	bot, exists := m.bots[platformKey]
	if !exists {
		// 幂等：机器人不存在视为已停止
		logger.Infof("[%s] 机器人不存在，无需停止", platform)
		return nil
	}

	if err := bot.Stop(); err != nil {
		return fmt.Errorf("停止机器人失败: %v", err)
	}

	delete(m.bots, platformKey)
	logger.Infof("[%s] 自动回复机器人已停止", platform)
	return nil
}

// GetBot 获取指定平台的机器人
func (m *AutoReplyManager) GetBot(platform Platform) (*AutoReplyBot, error) {
	m.botsMux.RLock()
	defer m.botsMux.RUnlock()

	bot, exists := m.bots[string(platform)]
	if !exists {
		return nil, fmt.Errorf("[%s] 机器人不存在", platform)
	}
	return bot, nil
}

// GetAllBots 获取所有机器人
func (m *AutoReplyManager) GetAllBots() map[string]*AutoReplyBot {
	m.botsMux.RLock()
	defer m.botsMux.RUnlock()

	// 返回副本避免竞态条件
	result := make(map[string]*AutoReplyBot)
	for k, v := range m.bots {
		result[k] = v
	}
	return result
}

// IsBotRunning 检查机器人是否运行中
func (m *AutoReplyManager) IsBotRunning(platform Platform) bool {
	m.botsMux.RLock()
	defer m.botsMux.RUnlock()

	bot, exists := m.bots[string(platform)]
	if !exists {
		return false
	}
	return bot.IsRunning()
}

// SetHeadless 设置指定平台的无头模式
func (m *AutoReplyManager) SetHeadless(platform string, headless bool) {
	m.botsMux.Lock()
	defer m.botsMux.Unlock()

	m.headless[platform] = headless

	// 如果该平台已有运行的机器人，更新其设置
	if bot, exists := m.bots[platform]; exists {
		bot.SetHeadless(headless)
	}
}

// GetHeadless 获取指定平台的无头模式
func (m *AutoReplyManager) GetHeadless(platform string) bool {
	m.botsMux.RLock()
	defer m.botsMux.RUnlock()

	if headless, exists := m.headless[platform]; exists {
		return headless
	}
	return true // 默认使用无头模式
}

// StopAllBots 停止所有机器人
func (m *AutoReplyManager) StopAllBots() {
	m.botsMux.Lock()
	defer m.botsMux.Unlock()

	for platform, bot := range m.bots {
		if bot.IsRunning() {
			if err := bot.Stop(); err != nil {
				logger.Errorf("[%s] 停止机器人失败: %v", platform, err)
			}
		}
		// 确保浏览器资源被释放
		if assistant := bot.GetAssistant(); assistant != nil {
			assistant.Close()
		}
	}

	// 清空机器人映射
	for platform := range m.bots {
		delete(m.bots, platform)
	}

	logger.Info("所有自动回复机器人已停止")
}

// GetStatus 获取管理器状态
func (m *AutoReplyManager) GetStatus() map[string]any {
	m.botsMux.RLock()
	defer m.botsMux.RUnlock()

	status := make(map[string]any)
	status["headless_settings"] = m.headless // 显示各平台的无头模式设置
	status["total_bots"] = len(m.bots)

	botsStatus := make(map[string]any)
	for platform, bot := range m.bots {
		botsStatus[platform] = map[string]any{
			"running":  bot.IsRunning(),
			"platform": bot.GetPlatform(),
			"account":  bot.GetAccount(),
			"headless": bot.IsHeadless(),
		}
	}
	status["bots"] = botsStatus

	return status
}
