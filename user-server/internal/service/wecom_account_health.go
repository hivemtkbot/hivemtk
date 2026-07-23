package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"

	"marketing/internal/model"
)

// WeComAccountHealthService 企微账号健康度服务
type WeComAccountHealthService struct {
	db *gorm.DB
}

// 常量定义
const (
	WeComHealthScoreNormal		= 100
	WeComHealthScoreWarning		= 70
	WeComHealthScoreCritical	= 40
	WeComHealthScoreBanned		= 0

	WeComRiskNormal		= "normal"
	WeComRiskWarning	= "warning"
	WeComRiskCritical	= "critical"
	WeComRiskBanned		= "banned"

	WeComLoginOnline	= "online"
	WeComLoginOffline	= "offline"
	WeComLoginBanned	= "banned"

	// 默认权重配置
	WeComDefaultWeight	= 100
	// 错误率超阈值后自动降权
	WeComErrorRateDegradeThreshold	= 0.3
	// 配额超阈值后自动降权
	WeComQuotaDegradeThreshold	= 0.9
)

// 错误定义
var (
	ErrWeComAccountNotFound		= errors.New("account not found")
	ErrWeComInvalidAccountID	= errors.New("invalid account_id")
	ErrWeComQuotaExceeded		= errors.New("daily quota exceeded")
	ErrWeComAccountBanned		= errors.New("account is banned")
	ErrWeComAccountOffline		= errors.New("account is offline")
	ErrWeComHealthNotFound		= errors.New("health record not found")
)

// NewWeComAccountHealthService 创建账号健康度服务
func NewWeComAccountHealthService(db *gorm.DB) *WeComAccountHealthService {
	return &WeComAccountHealthService{db: db}
}

// ReportHealthRequest 上报健康度
type ReportHealthRequest struct {
	AccountID	uint		`json:"account_id"`
	Platform	string		`json:"platform"`
	LoginState	string		`json:"login_state"`
	QuotaUsed	int		`json:"quota_used"`
	QuotaTotal	int		`json:"quota_total"`
	SuccessRate	float64		`json:"success_rate"`
	ErrorCount	int		`json:"error_count"`
	LastError	string		`json:"last_error"`
	Metrics		map[string]any	`json:"metrics"`
}

// ReportHealth 上报账号健康度，自动计算健康分与风险等级
func (s *WeComAccountHealthService) ReportHealth(ctx context.Context, req *ReportHealthRequest) (*model.WeComAccountHealth, error) {
	if s.db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	// 私域独立部署：无 merchant_id 校验
	if req.AccountID == 0 {
		return nil, ErrWeComInvalidAccountID
	}
	platform := req.Platform
	if platform == "" {
		platform = "wecom"
	}

	// 计算配额使用率
	quotaRate := 0.0
	if req.QuotaTotal > 0 {
		quotaRate = float64(req.QuotaUsed) / float64(req.QuotaTotal)
	}

	// 计算健康分
	score := computeHealthScore(req.LoginState, quotaRate, req.SuccessRate, req.ErrorCount)

	// 计算风险等级
	risk := computeRiskLevel(score, req.LoginState)

	now := time.Now()
	rec := &model.WeComAccountHealth{

		AccountID:	req.AccountID,
		Platform:	platform,
		HealthScore:	score,
		RiskLevel:	risk,
		LoginState:	req.LoginState,
		QuotaUsed:	req.QuotaUsed,
		QuotaTotal:	req.QuotaTotal,
		QuotaUsageRate:	quotaRate * 100,
		SuccessRate:	req.SuccessRate,
		ErrorCount:	req.ErrorCount,
		LastError:	req.LastError,
		Metrics:	model.JSONMap(req.Metrics),
		ReportedAt:	now,
	}
	if rec.Metrics == nil {
		rec.Metrics = model.JSONMap{}
	}
	if err := s.db.Create(rec).Error; err != nil {
		return nil, err
	}

	// 同步更新账号主表
	s.syncAccountState(ctx, req.AccountID, req.LoginState, score, risk, quotaRate, req.LastError)

	return rec, nil
}

// GetLatestHealth 获取最新健康度
func (s *WeComAccountHealthService) GetLatestHealth(ctx context.Context, accountID uint) (*model.WeComAccountHealth, error) {
	if s.db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	var rec model.WeComAccountHealth
	// 私域独立部署：无 merchant_id 字段
	if err := s.db.WithContext(ctx).Where("account_id = ?", accountID).Order("reported_at DESC").First(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWeComHealthNotFound
		}
		return nil, err
	}
	return &rec, nil
}

// ListHealthHistory 列出健康度历史
func (s *WeComAccountHealthService) ListHealthHistory(ctx context.Context, accountID uint, page, pageSize int) ([]model.WeComAccountHealth, int64, error) {
	if s.db == nil {
		return nil, 0, nil
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	var total int64
	// 私域独立部署：无 merchant_id 字段
	if err := s.db.Model(&model.WeComAccountHealth{}).
		Where("account_id = ?", accountID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.WeComAccountHealth
	err := s.db.Model(&model.WeComAccountHealth{}).
		Where("account_id = ?", accountID).
		Order("reported_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&list).Error
	return list, total, err
}

// GetRiskAccounts 列出风险账号
func (s *WeComAccountHealthService) GetRiskAccounts(ctx context.Context) ([]model.WeComAccount, error) {
	if s.db == nil {
		return nil, nil
	}
	var accounts []model.WeComAccount
	if err := s.db.WithContext(ctx).Where("risk_level IN ?",
		[]string{WeComRiskWarning, WeComRiskCritical, WeComRiskBanned}).
		Find(&accounts).Error; err != nil {
		return nil, err
	}
	return accounts, nil
}

// SelectHealthyAccount 路由选号 - 选出最佳账号
func (s *WeComAccountHealthService) SelectHealthyAccount(ctx context.Context) (*model.WeComAccount, error) {
	if s.db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	var accounts []model.WeComAccount
	if err := s.db.WithContext(ctx).Where("risk_level IN ? AND login_state != ? AND login_state != ?",
		[]string{WeComRiskNormal, WeComRiskWarning}, WeComLoginBanned, WeComLoginOffline).
		Find(&accounts).Error; err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, ErrWeComAccountNotFound
	}
	// 按健康度与权重排序
	sort.Slice(accounts, func(i, j int) bool {
		scoreI := accountHealthFromModel(&accounts[i])
		scoreJ := accountHealthFromModel(&accounts[j])
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return accounts[i].Weight > accounts[j].Weight
	})
	// 配额使用率最低优先
	sort.SliceStable(accounts, func(i, j int) bool {
		rateI := 0.0
		if accounts[i].DailyMsgQuota > 0 {
			rateI = float64(accounts[i].DailyMsgUsed) / float64(accounts[i].DailyMsgQuota)
		}
		rateJ := 0.0
		if accounts[j].DailyMsgQuota > 0 {
			rateJ = float64(accounts[j].DailyMsgUsed) / float64(accounts[j].DailyMsgQuota)
		}
		return rateI < rateJ
	})
	best := accounts[0]
	return &best, nil
}

// ConsumeQuota 消耗配额
func (s *WeComAccountHealthService) ConsumeQuota(ctx context.Context, accountID uint, count int) error {
	if s.db == nil {
		return nil
	}
	if count <= 0 {
		return nil
	}
	// 检查账号状态
	var acc model.WeComAccount
	if err := s.db.WithContext(ctx).First(&acc, accountID).Error; err != nil {
		return err
	}
	if acc.Status != 1 {
		return ErrWeComAccountBanned
	}
	if acc.LoginState == WeComLoginBanned {
		return ErrWeComAccountBanned
	}
	if acc.DailyMsgUsed+count > acc.DailyMsgQuota {
		return ErrWeComQuotaExceeded
	}
	return s.db.Model(&model.WeComAccount{}).
		Where("id = ?", accountID).
		Updates(map[string]any{
			"daily_msg_used":	acc.DailyMsgUsed + count,
			"total_sent":		acc.TotalSent + int64(count),
			"last_active_at":	time.Now(),
		}).Error
}

// ResetDailyQuota 重置日配额（每日凌晨）
func (s *WeComAccountHealthService) ResetDailyQuota(ctx context.Context) (int64, error) {
	if s.db == nil {
		return 0, nil
	}
	now := time.Now()
	// 单租户部署：无 merchant_id 字段，重置所有账号的日配额
	// GORM 批量 Updates 必须带 WHERE，使用 1=1 占位（语义上等同无条件更新）
	res := s.db.Model(&model.WeComAccount{}).
		Where("1 = 1").
		Updates(map[string]any{
			"daily_msg_used":	0,
			"quota_reset_at":	now,
		})
	return res.RowsAffected, res.Error
}

// AccountHealthSummary 账号健康概览
type AccountHealthSummary struct {
	TotalAccounts	int				`json:"total_accounts"`
	OnlineCount	int				`json:"online_count"`
	OfflineCount	int				`json:"offline_count"`
	BannedCount	int				`json:"banned_count"`
	WarningCount	int				`json:"warning_count"`
	CriticalCount	int				`json:"critical_count"`
	AvgScore	float64				`json:"avg_score"`
	TotalQuota	int				`json:"total_quota"`
	TotalUsed	int				`json:"total_used"`
	Accounts	[]AccountHealthSummaryEntry	`json:"accounts"`
	RiskAccounts	[]model.WeComAccount		`json:"risk_accounts"`
}

// AccountHealthSummaryEntry 单账号健康概览
type AccountHealthSummaryEntry struct {
	AccountID	uint	`json:"account_id"`
	CorpID		string	`json:"corp_id"`
	LoginState	string	`json:"login_state"`
	RiskLevel	string	`json:"risk_level"`
	HealthScore	int	`json:"health_score"`
	QuotaUsageRate	float64	`json:"quota_usage_rate"`
	SuccessRate	float64	`json:"success_rate"`
	TotalSent	int64	`json:"total_sent"`
	TotalReceived	int64	`json:"total_received"`
	ErrorCount	int	`json:"error_count"`
}

// GetHealthSummary 汇总账号健康度
func (s *WeComAccountHealthService) GetHealthSummary(ctx context.Context) (*AccountHealthSummary, error) {
	if s.db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	var accounts []model.WeComAccount
	if err := s.db.WithContext(ctx).Find(&accounts).Error; err != nil {
		return nil, err
	}

	summary := &AccountHealthSummary{
		TotalAccounts: len(accounts),
	}
	if len(accounts) == 0 {
		return summary, nil
	}
	scoreSum := 0
	for _, a := range accounts {
		switch a.LoginState {
		case WeComLoginOnline:
			summary.OnlineCount++
		case WeComLoginBanned:
			summary.BannedCount++
		default:
			summary.OfflineCount++
		}
		switch a.RiskLevel {
		case WeComRiskWarning:
			summary.WarningCount++
		case WeComRiskCritical:
			summary.CriticalCount++
		case WeComRiskBanned:
			summary.BannedCount++
		}
		summary.TotalQuota += a.DailyMsgQuota
		summary.TotalUsed += a.DailyMsgUsed
		scoreSum += a.Weight
		quotaRate := 0.0
		if a.DailyMsgQuota > 0 {
			quotaRate = float64(a.DailyMsgUsed) / float64(a.DailyMsgQuota)
		}
		summary.Accounts = append(summary.Accounts, AccountHealthSummaryEntry{
			AccountID:	a.ID,
			CorpID:		a.CorpID,
			LoginState:	a.LoginState,
			RiskLevel:	a.RiskLevel,
			HealthScore:	a.Weight,
			QuotaUsageRate:	quotaRate,
			TotalSent:	a.TotalSent,
			TotalReceived:	a.TotalReceived,
			ErrorCount:	a.ErrorCount,
		})
	}
	summary.AvgScore = float64(scoreSum) / float64(len(accounts))

	riskAccounts, _ := s.GetRiskAccounts(ctx)
	summary.RiskAccounts = riskAccounts
	return summary, nil
}

// computeHealthScore 计算账号健康分
func computeHealthScore(loginState string, quotaRate, successRate float64, errorCount int) int {
	score := 100
	if loginState == WeComLoginBanned {
		return WeComHealthScoreBanned
	}
	if loginState == WeComLoginOffline {
		score -= 50
	}
	// 配额使用率
	if quotaRate > 0.95 {
		score -= 25
	} else if quotaRate > WeComQuotaDegradeThreshold {
		score -= 15
	} else if quotaRate > 0.7 {
		score -= 5
	}
	// 成功率
	if successRate < 50 {
		score -= 30
	} else if successRate < 80 {
		score -= 15
	} else if successRate < 95 {
		score -= 5
	}
	// 错误数
	if errorCount > 50 {
		score -= 35
	} else if errorCount > 20 {
		score -= 15
	} else if errorCount > 5 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// computeRiskLevel 计算风险等级
func computeRiskLevel(score int, loginState string) string {
	if loginState == WeComLoginBanned {
		return WeComRiskBanned
	}
	switch {
	case score >= WeComHealthScoreNormal:
		return WeComRiskNormal
	case score >= WeComHealthScoreWarning:
		return WeComRiskWarning
	case score >= WeComHealthScoreCritical:
		return WeComRiskCritical
	default:
		return WeComRiskCritical
	}
}

// syncAccountState 同步账号主表状态
func (s *WeComAccountHealthService) syncAccountState(ctx context.Context, accountID uint, loginState string, score int, risk string, quotaRate float64, lastErr string) {
	if s.db == nil {
		return
	}
	updates := map[string]any{
		"login_state":		loginState,
		"risk_level":		risk,
		"weight":		score,
		"last_active_at":	time.Now(),
	}
	if lastErr != "" {
		updates["last_error_at"] = time.Now()
		updates["last_error_msg"] = lastErr
		// 原实现误用 s.db.Raw(...) 把 *gorm.DB 当作 error_count 的值塞进 updates map，
		// 既不执行 SQL 也产生类型污染。已删除 dead code，直接使用 gorm.Expr 自增。（R5 修复）
		updates["error_count"] = gorm.Expr("error_count + 1")
	}
	if quotaRate > WeComQuotaDegradeThreshold && risk == WeComRiskNormal {
		updates["risk_level"] = WeComRiskWarning
	}
	s.db.Model(&model.WeComAccount{}).
		// 私域独立部署：无 merchant_id 字段
		Where("id = ?", accountID).
		Updates(updates)
}

func accountHealthFromModel(a *model.WeComAccount) int {
	if a == nil {
		return 0
	}
	return a.Weight
}

// 全局实例
var (
	wecomHealthOnce		sync.Once
	wecomHealthInstance	*WeComAccountHealthService
)

// GetWeComAccountHealthService 获取账号健康度服务
//
// 当全局实例尚未初始化时（如：单元测试、嵌入式使用），自动以 nil db 兜底构造一个实例
// 避免 panic。生产路径仍应在启动时调用 InitWeComAccountHealthService(db) 注入真实 db。
func GetWeComAccountHealthService() *WeComAccountHealthService {
	if wecomHealthInstance == nil {
		// 测试场景：构造一个 db=nil 的实例用于占位访问
		wecomHealthInstance = &WeComAccountHealthService{}
	}
	return wecomHealthInstance
}

// InitWeComAccountHealthService 初始化账号健康度服务
func InitWeComAccountHealthService(db *gorm.DB) *WeComAccountHealthService {
	wecomHealthOnce.Do(func() {
		wecomHealthInstance = NewWeComAccountHealthService(db)
	})
	return wecomHealthInstance
}
