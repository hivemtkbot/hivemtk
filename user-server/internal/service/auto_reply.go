package service

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"marketing/internal/aiagent/agent/browser"
	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"

	"github.com/gin-gonic/gin"

	"gorm.io/gorm"
)

type AutoReplyService struct {
	merchantRepo repository.AccountRepository
	accountRepo  *repository.AutoReplyAccountRepository
	ruleRepo     *repository.AutoReplyRuleRepository
	logRepo      *repository.AutoReplyLogRepository
}

func NewAutoReplyService(db *gorm.DB) *AutoReplyService {
	return &AutoReplyService{
		merchantRepo: repository.NewAccountRepositoryWithDB(db),
		accountRepo:  repository.NewAutoReplyAccountRepository(db),
		ruleRepo:     repository.NewAutoReplyRuleRepositoryWithDB(db),
		logRepo:      repository.NewAutoReplyLogRepository(db),
	}
}

// NewAutoReplyServiceAuto 创建自动回复服务（五层架构合规：仓储层统一封装 DB 获取入口）
//
// 用于 service 构造函数内不允许直接获取数据库句柄的场景
// （例如 tiktok_auto_reply.go），仓储层内部已用全局 DB 初始化。
func NewAutoReplyServiceAuto() *AutoReplyService {
	return &AutoReplyService{
		merchantRepo: repository.NewAccountRepository(),
		accountRepo:  repository.NewAutoReplyAccountRepositoryAuto(),
		ruleRepo:     repository.NewAutoReplyRuleRepository(),
		logRepo:      repository.NewAutoReplyLogRepositoryAuto(),
	}
}

// GetDB 已删除（五层架构治理：service 不再暴露 *gorm.DB 给 controller）
// 历史调用方请改用 repository 层方法。

func (s *AutoReplyService) ListAccounts(ctx context.Context, platform string, userID uint) ([]model.AutoReplyAccount, error) {
	return s.accountRepo.ListByPlatformAndUser(ctx, platform, userID)
}

func (s *AutoReplyService) UpsertAccount(ctx context.Context, a *model.AutoReplyAccount) error {
	existing, err := s.accountRepo.FindByUserAndPlatformAndUsername(ctx, a.UserID, a.Platform, a.Username)
	if err == nil && existing != nil {
		existing.Cookie = a.Cookie
		existing.IsActive = a.IsActive
		existing.LoginAt = a.LoginAt
		return s.accountRepo.Save(ctx, existing)
	}
	return s.accountRepo.Create(ctx, a)
}

// SaveCookies 保存账号 Cookie
//
// 安全修复：原签名 SaveCookies(id, cookie) 缺少 userID 参数，
// 任意已认证用户可通过 /api/autoreply/accounts/:id/cookies 覆盖任意账号 Cookie（IDOR）。
// 现要求传入 userID 并校验 account.UserID == userID，越权访问返回 ErrAccountNotOwned。
func (s *AutoReplyService) SaveCookies(ctx context.Context, id uint, cookie string, userID uint) error {
	// 获取现有的账号记录
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// 所有权校验：仅账号所有者可修改 Cookie
	if account.UserID != userID {
		return ErrAccountNotOwned
	}

	account.Cookie = cookie

	return s.accountRepo.UpdateCookieByID(ctx, id, account.Cookie)
}

// ErrAccountNotOwned 账号不属于当前用户（IDOR 防护）
var ErrAccountNotOwned = errors.New("auto reply account does not belong to current user")

func (s *AutoReplyService) GetRule(ctx context.Context, platform string, userID uint) (*model.AutoReplyRule, error) {
	return s.ruleRepo.FindByPlatformAndUser(ctx, platform, userID)
}

func (s *AutoReplyService) SaveRule(ctx context.Context, rule *model.AutoReplyRule) error {
	existing, err := s.ruleRepo.FindExistingByPlatformAndUser(ctx, rule.Platform, rule.UserID)
	if err == nil && existing != nil {
		existing.Keywords = rule.Keywords
		existing.ReplyContent = rule.ReplyContent
		existing.Frequency = rule.Frequency
		existing.DailyLimit = rule.DailyLimit
		existing.StartTime = rule.StartTime // 添加时间段字段
		existing.EndTime = rule.EndTime     // 添加时间段字段
		existing.IsActive = rule.IsActive
		existing.IsRagEnabled = rule.IsRagEnabled
		existing.RagProductID = rule.RagProductID
		return s.ruleRepo.Save(ctx, existing)
	}
	return s.ruleRepo.Create(ctx, rule)
}

func (s *AutoReplyService) ListRecentLogs(ctx context.Context, platform string, userID uint, page, pageSize int) ([]model.AutoReplyLog, int64, error) {
	cutoff := time.Now().Add(-72 * time.Hour)
	return s.logRepo.ListRecentByPlatformAndUser(ctx, platform, userID, page, pageSize, cutoff)
}

func (s *AutoReplyService) AppendLog(userID, accountID, ruleID uint, platform, target, reply, status, errMsg string) error {
	item := &model.AutoReplyLog{UserID: userID, AccountID: accountID, RuleID: ruleID, Platform: platform, TargetContent: target, ReplyContent: reply, Status: status, ErrorMsg: errMsg, CreatedAt: time.Now()}
	return s.logRepo.Create(context.Background(), item)
}

// StartLoginBrowser 启动本地浏览器打开登录页面并在登录后提取 Cookie 保存
// 注意：需要服务器具备可用的 Chromium/Chrome 环境
func (s *AutoReplyService) StartLoginBrowser(ctx context.Context, userID uint, platform, username string, accountID uint, headless bool) {
	go func() {
		ctx := context.Background()
		logger.Infof("启动登录浏览器 - 平台: %s, 用户: %s, 无头模式: %v", platform, username, headless)

		a, err := browser.NewAssistant(browser.Options{Headless: headless})
		if err != nil {
			logger.Errorf("创建浏览器助手失败: %v", err)
			return
		}
		defer a.Close()
		p := browser.Platform(platform)
		loginURL := browser.LoginURL(p)
		logger.Infof("导航到登录页面: %s", loginURL)
		if err := a.Navigate(loginURL); err != nil {
			logger.Errorf("导航到登录页面失败: %v", err)
			return
		}
		logger.Infof("等待用户登录，超时时间: 5分钟")
		cookie, ok := a.WaitAuthCookieHeader(p, 5*time.Minute)
		if ok {
			account, err := s.accountRepo.GetByID(ctx, accountID)
			if err == nil {
				account.Cookie = cookie
				if err := s.accountRepo.UpdateCookieByID(ctx, accountID, account.Cookie); err != nil {
					logger.Errorf("保存Cookie失败: %v", err)
				}
			}
		} else {
			logger.Errorf("平台 %s 用户 %s 登录超时或失败", platform, username)
		}
	}()
}

func (s *AutoReplyService) DeleteAccount(ctx context.Context, id uint, userID uint) error {
	return s.accountRepo.DeleteByIDAndUser(ctx, id, userID)
}

// 新增方法 - 规则管理
func (s *AutoReplyService) ListRules(ctx context.Context, req *dto.AutoReplyRuleListRequest) ([]model.AutoReplyRule, int64, error) {
	return s.ruleRepo.ListWithFilters(ctx, req.Platform, req.UserID, req.IsActive, req.Page, req.PageSize)
}

func (s *AutoReplyService) CreateRule(ctx context.Context, req *dto.AutoReplyRuleRequest) (*model.AutoReplyRule, error) {
	rule := &model.AutoReplyRule{
		UserID:       req.UserID,
		Platform:     req.Platform,
		Keywords:     req.Keywords,
		ReplyContent: req.ReplyContent,
		Frequency:    req.Frequency,
		DailyLimit:   req.DailyLimit,
		StartTime:    req.StartTime, // 添加时间段字段
		EndTime:      req.EndTime,   // 添加时间段字段
		IsActive:     req.IsActive,
		IsRagEnabled: req.IsRagEnabled,
		RagProductID: req.RagProductID,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.ruleRepo.Create(ctx, rule); err != nil {
		return nil, err
	}
	return rule, nil
}

func (s *AutoReplyService) UpdateRule(ctx context.Context, id uint, req *dto.AutoReplyRuleRequest) (*model.AutoReplyRule, error) {
	rule, err := s.ruleRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	rule.Keywords = req.Keywords
	rule.ReplyContent = req.ReplyContent
	rule.Frequency = req.Frequency
	rule.DailyLimit = req.DailyLimit
	rule.StartTime = req.StartTime // 添加时间段字段
	rule.EndTime = req.EndTime     // 添加时间段字段
	rule.IsActive = req.IsActive
	rule.IsRagEnabled = req.IsRagEnabled
	rule.RagProductID = req.RagProductID
	rule.UpdatedAt = time.Now()

	if err := s.ruleRepo.Save(ctx, rule); err != nil {
		return nil, err
	}
	return rule, nil
}

func (s *AutoReplyService) DeleteRule(ctx context.Context, id uint) error {
	return s.ruleRepo.DeleteByID(ctx, id)
}

// 关键词匹配测试
func (s *AutoReplyService) TestMatching(ctx context.Context, platform, message string, userID uint) (*model.AutoReplyRule, error) {
	rules, err := s.ruleRepo.ListByPlatformAndUserActive(ctx, platform, userID)
	if err != nil {
		return nil, err
	}

	// 将消息转换为小写以便进行不区分大小写的匹配
	lowerMessage := strings.ToLower(message)

	for _, rule := range rules {
		// 检查时间段限制
		if !s.isWithinTimeRange(ctx, rule) {
			continue
		}

		// 检查每日频率限制
		if !s.hasRemainingDailyQuota(ctx, rule, userID) {
			continue
		}

		keywords := strings.Split(rule.Keywords, ",")
		for _, keyword := range keywords {
			trimmedKeyword := strings.TrimSpace(keyword)
			if trimmedKeyword == "" {
				continue
			}

			// 1. 精确匹配
			if strings.Contains(lowerMessage, strings.ToLower(trimmedKeyword)) {
				return &rule, nil
			}

			// 2. 模糊匹配（支持通配符）
			if isFuzzyMatch(lowerMessage, trimmedKeyword) {
				return &rule, nil
			}

			// 3. 正则表达式匹配（如果关键词以/开头和结尾）
			if isRegexMatch(message, trimmedKeyword) {
				return &rule, nil
			}
		}
	}
	return nil, nil
}

// 模糊匹配函数（支持通配符）
func isFuzzyMatch(message, pattern string) bool {
	// 如果模式包含通配符字符，则进行通配符匹配
	if strings.Contains(pattern, "*") || strings.Contains(pattern, "?") {
		// 简单的通配符匹配实现
		// 将通配符模式转换为正则表达式
		escapedPattern := regexp.QuoteMeta(pattern)
		// 替换通配符
		escapedPattern = strings.ReplaceAll(escapedPattern, "\\*", ".*")
		escapedPattern = strings.ReplaceAll(escapedPattern, "\\?", ".")

		// 编译正则表达式并匹配
		matched, err := regexp.MatchString("(?i)"+escapedPattern, message)
		return err == nil && matched
	}
	return false
}

// 正则表达式匹配函数
func isRegexMatch(message, pattern string) bool {
	if len(pattern) > 1 && pattern[0] == '/' && pattern[len(pattern)-1] == '/' {
		// 移除首尾的斜杠
		regexPattern := pattern[1 : len(pattern)-1]
		matched, err := regexp.MatchString(regexPattern, message)
		return err == nil && matched
	}
	return false
}

// 检查是否有剩余的日配额
func (s *AutoReplyService) hasRemainingDailyQuota(ctx context.Context, rule model.AutoReplyRule, userID uint) bool {
	if rule.DailyLimit <= 0 {
		return true // 如果没有设置日限制，则不限制
	}

	// 计算今天已发送的消息数量
	todayStart := time.Now().Truncate(24 * time.Hour)
	count, err := s.logRepo.CountByUserAndRuleSince(ctx, userID, rule.ID, todayStart)

	if err != nil {
		logger.Errorf("查询自动回复日志失败: %v", err)
		return true // 如果查询出错，默认允许发送
	}

	// 检查是否超过日限制
	return int(count) < rule.DailyLimit
}

// 检查是否在指定的时间范围内
func (s *AutoReplyService) isWithinTimeRange(ctx context.Context, rule model.AutoReplyRule) bool {
	// 如果没有设置时间段，则始终允许
	if rule.StartTime == nil || rule.EndTime == nil {
		return true
	}

	startTimeStr := *rule.StartTime
	endTimeStr := *rule.EndTime

	// 解析开始时间和结束时间
	currentTime := time.Now()
	currentHour := currentTime.Hour()
	currentMinute := currentTime.Minute()

	// 解析开始时间 (HH:MM格式)
	startParts := strings.Split(startTimeStr, ":")
	if len(startParts) != 2 {
		return true // 如果时间格式不正确，则允许
	}

	endParts := strings.Split(endTimeStr, ":")
	if len(endParts) != 2 {
		return true // 如果时间格式不正确，则允许
	}

	// 使用strconv.ParseInt解析小时和分钟
	startHour, err1 := strconv.ParseInt(startParts[0], 10, 64)
	startMinute, err2 := strconv.ParseInt(startParts[1], 10, 64)
	if err1 != nil || err2 != nil {
		return true // 如果解析失败，则允许
	}

	endHour, err3 := strconv.ParseInt(endParts[0], 10, 64)
	endMinute, err4 := strconv.ParseInt(endParts[1], 10, 64)
	if err3 != nil || err4 != nil {
		return true // 如果解析失败，则允许
	}

	// 构建当前时间、开始时间和结束时间的总分钟数
	currentTotalMinutes := currentHour*60 + currentMinute
	startTotalMinutes := int(startHour)*60 + int(startMinute)
	endTotalMinutes := int(endHour)*60 + int(endMinute)

	// 判断当前时间是否在时间段内
	if startTotalMinutes <= endTotalMinutes {
		// 正常情况（例如 09:00 - 18:00）
		return currentTotalMinutes >= startTotalMinutes && currentTotalMinutes <= endTotalMinutes
	}
	return false
}

// 模拟消息处理
func (s *AutoReplyService) SimulateMessage(ctx context.Context, platform, message, sender string, userID, accountID uint) (*model.AutoReplyLog, error) {
	rule, err := s.TestMatching(ctx, platform, message, userID)
	if err != nil {
		return nil, err
	}

	logEntry := &model.AutoReplyLog{
		UserID:        userID,
		AccountID:     accountID,
		RuleID:        0,
		Platform:      platform,
		TargetContent: message,
		ReplyContent:  "",
		Status:        "no_match",
		ErrorMsg:      "",
		CreatedAt:     time.Now(),
	}

	if rule != nil {
		logEntry.RuleID = rule.ID
		logEntry.ReplyContent = rule.ReplyContent
		logEntry.Status = "matched"
	}

	if err := s.logRepo.Create(ctx, logEntry); err != nil {
		return nil, err
	}

	return logEntry, nil
}

// 批量匹配测试
func (s *AutoReplyService) TestBatchMatching(ctx context.Context, platform string, messages []string, userID, accountID uint) ([]model.AutoReplyLog, error) {
	var results []model.AutoReplyLog

	for _, message := range messages {
		logEntry, err := s.SimulateMessage(ctx, platform, message, "test_sender", userID, accountID)
		if err != nil {
			continue
		}
		results = append(results, *logEntry)
	}

	return results, nil
}

// 速率限制测试
func (s *AutoReplyService) TestRateLimit(ctx context.Context, platform string, userID, accountID uint, testCount int) ([]model.RateLimitTestResult, error) {
	var results []model.RateLimitTestResult

	for i := 0; i < testCount; i++ {
		allowed := true
		errorMsg := ""

		// 简单的速率限制逻辑（每5秒最多3次）
		if i > 0 && i%3 == 0 {
			allowed = false
			errorMsg = "rate limited"
		}

		result := model.RateLimitTestResult{
			Platform:  platform,
			UserID:    userID,
			AccountID: accountID,
			TestID:    i + 1,
			Allowed:   allowed,
			ErrorMsg:  errorMsg,
			Timestamp: time.Now(),
		}
		results = append(results, result)
	}

	return results, nil
}

// 重置每日限制 - 删除当天该账户的速率限制记录（AutoReplyLog）
func (s *AutoReplyService) ResetDailyLimit(ctx context.Context, platform string, userID, accountID uint) error {
	todayStart := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, time.Now().Location())
	return s.logRepo.DeleteSinceByFilters(ctx, todayStart, platform, userID, accountID)
}

// 获取速率限制统计 - 从 AutoReplyLog 表查询当天真实发送量，从 AutoReplyRule 获取限额
func (s *AutoReplyService) GetRateLimitStats(ctx context.Context, platform string, userID, accountID uint) (gin.H, error) {
	todayStart := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, time.Now().Location())

	// 查询当天已发送数量
	dailySent, err := s.logRepo.CountSinceByFilters(ctx, todayStart, platform, userID, accountID)
	if err != nil {
		dailySent = 0
	}

	// 从规则表获取每日限额（取该平台/账户配置的最大限额，默认 100）
	var dailyLimit int64 = 100
	rule, err := s.ruleRepo.FindTopDailyLimit(ctx, platform, userID)
	if err == nil && rule != nil && rule.DailyLimit > 0 {
		dailyLimit = int64(rule.DailyLimit)
	}

	// 计算次日 0 点重置时间
	tomorrow := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day()+1, 0, 0, 0, 0, time.Now().Location())

	stats := gin.H{
		"platform":    platform,
		"user_id":     userID,
		"account_id":  accountID,
		"daily_sent":  dailySent,
		"daily_limit": dailyLimit,
		"reset_time":  tomorrow.Format("2006-01-02 15:04:05"),
	}
	return stats, nil
}

// 获取并发统计 - 从 AutoReplyLog 表查询当前活跃任务数
func (s *AutoReplyService) GetConcurrentStats(ctx context.Context, platform string, userID uint) (gin.H, error) {
	// 查询最近 5 分钟内的活跃任务数（近似并发数）
	fiveMinAgo := time.Now().Add(-5 * time.Minute)
	activeBots, err := s.logRepo.CountActiveAccountsSince(ctx, fiveMinAgo, platform, userID)
	if err != nil {
		activeBots = 0
	}

	// 查询待处理任务数（status=pending 的日志）
	queueSize, err := s.logRepo.CountPendingByFilters(ctx, platform, userID)
	if err != nil {
		queueSize = 0
	}

	// 最大并发数从配置读取（默认 5）
	maxConcurrent := int64(5)
	// account 表暂无 max_concurrent 字段，保持默认值

	stats := gin.H{
		"platform":       platform,
		"user_id":        userID,
		"active_bots":    activeBots,
		"max_concurrent": maxConcurrent,
		"queue_size":     queueSize,
	}
	return stats, nil
}

// GetStatistics 获取综合统计
//
// 五层架构合规：将原 controller 中的 DB 查询逻辑下沉到 service，
// controller 仅做参数解析与响应封装。
//
// 参数：
//   - platform: 平台过滤（空字符串表示不过滤）
//   - userID: 用户 ID（0 表示不过滤，由调用方保证语义）
//
// 返回包含 rule/log/today_log 计数的统计字典。
func (s *AutoReplyService) GetStatistics(ctx context.Context, platform string, userID uint) (gin.H, error) {
	// 规则总数
	ruleCount, err := s.ruleRepo.CountByFilters(ctx, platform, userID)
	if err != nil {
		return nil, err
	}

	// 日志总数
	logCount, err := s.logRepo.CountByFilters(ctx, platform, userID)
	if err != nil {
		return nil, err
	}

	// 今日日志数
	today := time.Now().Format("2006-01-02")
	todayLogCount, err := s.logRepo.CountByFiltersAndDate(ctx, platform, userID, today)
	if err != nil {
		return nil, err
	}

	stats := gin.H{
		"platform":         platform,
		"user_id":          userID,
		"total_rules":      ruleCount,
		"total_logs":       logCount,
		"today_logs":       todayLogCount,
		"active_platforms": []string{"douyin", "kuaishou", "xiaohongshu", "xianyu"},
	}
	return stats, nil
}
