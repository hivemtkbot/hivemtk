package service

import (
	"context"
	"errors"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ============================================================================
// AutoReplyService DTO 外观方法（保持原 model 签名方法不变）
// 集中处理原先 controller 直接访问 model.Account（商户账户）与 GetDB() 的逻辑。
// ============================================================================

// MerchantHeadlessSettings 商户账户各平台无头模式设置
type MerchantHeadlessSettings struct {
	Douyin		bool
	Kuaishou	bool
	Xiaohongshu	bool
	Xianyu		bool
}

// GetMerchantHeadlessSettings 读取商户账户的无头模式设置（不存在则返回默认 true）
func (s *AutoReplyService) GetMerchantHeadlessSettings(ctx context.Context,) (*MerchantHeadlessSettings, error) {
	var merchantAccount model.Account
	err := s.db.WithContext(ctx).First(&merchantAccount).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &MerchantHeadlessSettings{Douyin: true, Kuaishou: true, Xiaohongshu: true, Xianyu: true}, nil
	}
	if err != nil {
		return nil, err
	}
	return &MerchantHeadlessSettings{
		Douyin:		utils.GetBoolValue(merchantAccount.DouyinHeadless, true),
		Kuaishou:	utils.GetBoolValue(merchantAccount.KuaishouHeadless, true),
		Xiaohongshu:	utils.GetBoolValue(merchantAccount.XiaohongshuHeadless, true),
		Xianyu:		utils.GetBoolValue(merchantAccount.XianyuHeadless, true),
	}, nil
}

// SetMerchantHeadless 设置商户账户指定平台的无头模式
func (s *AutoReplyService) SetMerchantHeadless(ctx context.Context, platform string, headless bool) error {
	var account model.Account
	if err := s.db.WithContext(ctx).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			account = defaultMerchantAccount()
		} else {
			return err
		}
	}
	switch platform {
	case "douyin":
		account.DouyinHeadless = utils.BoolPtr(headless)
	case "kuaishou":
		account.KuaishouHeadless = utils.BoolPtr(headless)
	case "xiaohongshu":
		account.XiaohongshuHeadless = utils.BoolPtr(headless)
	case "xianyu":
		account.XianyuHeadless = utils.BoolPtr(headless)
	default:
		return errors.New("不支持的平台")
	}
	if account.ID == "" {
		if err := s.db.Create(&account).Error; err != nil {
			return err
		}
	} else {
		if err := s.db.Save(&account).Error; err != nil {
			return err
		}
	}
	return nil
}

// defaultMerchantAccount 创建默认商户账户
func defaultMerchantAccount() model.Account {
	return model.Account{
		TgBotToken:		"default_token",
		Price:			"0",
		GroupID:		0,
		ProxyEnableProxy:	false,
		ProxyProtoclo:		"http",
		ProxyHost:		"127.0.0.1",
		ProxyPort:		1080,
		DouyinHeadless:		utils.BoolPtr(true),
		KuaishouHeadless:	utils.BoolPtr(true),
		XiaohongshuHeadless:	utils.BoolPtr(true),
		XianyuHeadless:		utils.BoolPtr(true),
	}
}

// AutoReplyRuleSaveReq 保存自动回复规则请求
type AutoReplyRuleSaveReq struct {
	UserID		uint
	Platform	string
	Keywords	string
	ReplyContent	string
	Frequency	int
	DailyLimit	int
	StartTime	*string
	EndTime		*string
	IsActive	bool
	IsRagEnabled	bool
	RagProductID	*string
}

// SaveRuleDTO 根据请求 DTO 保存规则（构建 model.AutoReplyRule 逻辑下沉到 service）
func (s *AutoReplyService) SaveRuleDTO(ctx context.Context, req AutoReplyRuleSaveReq) error {
	rule := &model.AutoReplyRule{
		UserID:		req.UserID,
		Platform:	req.Platform,
		Keywords:	req.Keywords,
		ReplyContent:	req.ReplyContent,
		Frequency:	req.Frequency,
		DailyLimit:	req.DailyLimit,
		StartTime:	req.StartTime,
		EndTime:	req.EndTime,
		IsActive:	req.IsActive,
		IsRagEnabled:	req.IsRagEnabled,
		RagProductID:	req.RagProductID,
	}
	return s.SaveRule(ctx, rule)
}

// AutoReplyAccountCreateReq 创建自动回复账号请求
type AutoReplyAccountCreateReq struct {
	UserID		uint
	Platform	string
	Username	string
	Cookie		string
	IsActive	bool
	Headless	bool
	LoginAt		*time.Time
}

// UpsertAutoReplyAccount 根据请求 DTO 创建/更新自动回复账号，返回账号 ID
func (s *AutoReplyService) UpsertAutoReplyAccount(ctx context.Context, req AutoReplyAccountCreateReq) (uint, error) {
	item := &model.AutoReplyAccount{
		UserID:		req.UserID,
		Platform:	req.Platform,
		Username:	req.Username,
		Cookie:		req.Cookie,
		IsActive:	req.IsActive,
		Headless:	req.Headless,
		LoginAt:	req.LoginAt,
	}
	if err := s.UpsertAccount(ctx, item); err != nil {
		return 0, err
	}
	return item.ID, nil
}

// GetAutoReplyStatistics 综合统计（原先 controller 直接使用 GetDB().Model(&model.AutoReplyRule{}) 统计）
func (s *AutoReplyService) GetAutoReplyStatistics(ctx context.Context, platform string, userID uint) (gin.H, error) {
	var ruleCount int64
	ruleQuery := s.db.Model(&model.AutoReplyRule{})
	if platform != "" {
		ruleQuery = ruleQuery.Where("platform = ?", platform)
	}
	if userID > 0 {
		ruleQuery = ruleQuery.Where("user_id = ?", userID)
	}
	if err := ruleQuery.Count(&ruleCount).Error; err != nil {
		return nil, err
	}

	var logCount int64
	logQuery := s.db.Model(&model.AutoReplyLog{})
	if platform != "" {
		logQuery = logQuery.Where("platform = ?", platform)
	}
	if userID > 0 {
		logQuery = logQuery.Where("user_id = ?", userID)
	}
	if err := logQuery.Count(&logCount).Error; err != nil {
		return nil, err
	}

	today := time.Now().Format("2006-01-02")
	todayLogQuery := s.db.Model(&model.AutoReplyLog{}).Where("DATE(created_at) = ?", today)
	if platform != "" {
		todayLogQuery = todayLogQuery.Where("platform = ?", platform)
	}
	if userID > 0 {
		todayLogQuery = todayLogQuery.Where("user_id = ?", userID)
	}
	var todayLogCount int64
	if err := todayLogQuery.Count(&todayLogCount).Error; err != nil {
		return nil, err
	}

	return gin.H{
		"platform":		platform,
		"user_id":		userID,
		"total_rules":		ruleCount,
		"total_logs":		logCount,
		"today_logs":		todayLogCount,
		"active_platforms":	[]string{"douyin", "kuaishou", "xiaohongshu", "xianyu"},
	}, nil
}

// RateLimitTestResultDTO 速率限制测试结果（供 controller 序列化，避免直接引用 model）
type RateLimitTestResultDTO struct {
	Platform	string		`json:"platform"`
	UserID		uint		`json:"user_id"`
	AccountID	uint		`json:"account_id"`
	TestID		int		`json:"test_id"`
	Allowed		bool		`json:"allowed"`
	ErrorMsg	string		`json:"error_msg"`
	Timestamp	time.Time	`json:"timestamp"`
}

// TestRateLimitDTO 速率限制测试（返回 DTO 列表及统计）
func (s *AutoReplyService) TestRateLimitDTO(ctx context.Context, platform string, userID, accountID uint, testCount int) ([]RateLimitTestResultDTO, int, int, error) {
	results, err := s.TestRateLimit(ctx, platform, userID, accountID, testCount)
	if err != nil {
		return nil, 0, 0, err
	}
	dtos := make([]RateLimitTestResultDTO, 0, len(results))
	allowed, rateLimited := 0, 0
	for _, r := range results {
		dtos = append(dtos, RateLimitTestResultDTO{
			Platform:	r.Platform,
			UserID:		r.UserID,
			AccountID:	r.AccountID,
			TestID:		r.TestID,
			Allowed:	r.Allowed,
			ErrorMsg:	r.ErrorMsg,
			Timestamp:	r.Timestamp,
		})
		if r.Allowed {
			allowed++
		} else {
			rateLimited++
		}
	}
	return dtos, allowed, rateLimited, nil
}

// TestBatchMatchingDTO 批量匹配测试（返回 DTO 列表，避免 controller 直接引用 model.AutoReplyLog）
func (s *AutoReplyService) TestBatchMatchingDTO(ctx context.Context, platform string, messages []string, userID, accountID uint) ([]map[string]any, error) {
	results, err := s.TestBatchMatching(ctx, platform, messages, userID, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(results))
	for _, r := range results {
		out = append(out, map[string]any{
			"platform":	r.Platform,
			"user_id":	r.UserID,
			"account_id":	r.AccountID,
			"rule_id":	r.RuleID,
			"target":	r.TargetContent,
			"reply":	r.ReplyContent,
			"status":	r.Status,
			"error_msg":	r.ErrorMsg,
			"created_at":	r.CreatedAt,
		})
	}
	return out, nil
}
