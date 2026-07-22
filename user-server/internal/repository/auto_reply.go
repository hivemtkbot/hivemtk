package repository

import (
	"time"

	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
	"context"
)

// ============== AutoReplyAccountRepository ==============

// AutoReplyAccountRepository 自动回复账号仓库
type AutoReplyAccountRepository struct {
	db *gorm.DB
}

// NewAutoReplyAccountRepository 创建自动回复账号仓库实例
// 支持注入 *gorm.DB 以兼容内存测试
func NewAutoReplyAccountRepository(db *gorm.DB) *AutoReplyAccountRepository {
	return &AutoReplyAccountRepository{db: db}
}

// ListByPlatformAndUser 按平台与用户列出账号
func (r *AutoReplyAccountRepository) ListByPlatformAndUser(ctx context.Context, platform string, userID uint) ([]model.AutoReplyAccount, error) {
	var items []model.AutoReplyAccount
	err := r.db.Where("platform = ? AND user_id = ?", platform, userID).Find(&items).Error
	return items, err
}

// FindByUserAndPlatformAndUsername 通过用户/平台/用户名查找账号
func (r *AutoReplyAccountRepository) FindByUserAndPlatformAndUsername(ctx context.Context, userID uint, platform, username string) (*model.AutoReplyAccount, error) {
	var existing model.AutoReplyAccount
	err := r.db.Where("user_id = ? AND platform = ? AND username = ?", userID, platform, username).First(&existing).Error
	if err != nil {
		return nil, err
	}
	return &existing, nil
}

// GetByID 按 ID 获取账号
func (r *AutoReplyAccountRepository) GetByID(ctx context.Context, id uint) (*model.AutoReplyAccount, error) {
	var account model.AutoReplyAccount
	if err := r.db.First(&account, id).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

// Create 创建账号
func (r *AutoReplyAccountRepository) Create(ctx context.Context, account *model.AutoReplyAccount) error {
	return r.db.Create(account).Error
}

// Save 保存账号
func (r *AutoReplyAccountRepository) Save(ctx context.Context, account *model.AutoReplyAccount) error {
	return r.db.Save(account).Error
}

// UpdateCookieByID 按 ID 更新 Cookie 字段
func (r *AutoReplyAccountRepository) UpdateCookieByID(ctx context.Context, id uint, cookie string) error {
	return r.db.Model(&model.AutoReplyAccount{}).Where("id = ?", id).Update("cookie", cookie).Error
}

// DeleteByIDAndUser 按 ID 与用户删除账号（用户不匹配时不会删除）
func (r *AutoReplyAccountRepository) DeleteByIDAndUser(ctx context.Context, id, userID uint) error {
	return r.db.Where("id = ? AND user_id = ?", id, userID).Delete(&model.AutoReplyAccount{}).Error
}

// FirstByPlatformAndUser 取平台与用户下的首条账号
func (r *AutoReplyAccountRepository) FirstByPlatformAndUser(ctx context.Context, platform string, userID uint) (*model.AutoReplyAccount, error) {
	var account model.AutoReplyAccount
	if err := r.db.Model(&model.AutoReplyAccount{}).Where("platform = ? AND user_id = ?", platform, userID).First(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

// ============== AutoReplyRuleRepository ==============

// AutoReplyRuleRepository 自动回复规则仓库
type AutoReplyRuleRepository struct {
	db *gorm.DB
}

// NewAutoReplyRuleRepository 创建自动回复规则仓库实例(五层架构合规:无参,内部用 _db.GetDB())
func NewAutoReplyRuleRepository() *AutoReplyRuleRepository {
	return &AutoReplyRuleRepository{db: _db.GetDB()}
}

// NewAutoReplyRuleRepositoryWithDB 创建自动回复规则仓库实例(显式注入 db,兼容旧调用)
func NewAutoReplyRuleRepositoryWithDB(db *gorm.DB) *AutoReplyRuleRepository {
	return &AutoReplyRuleRepository{db: db}
}

// FindByPlatformAndUser 取平台与用户的最新一条规则
func (r *AutoReplyRuleRepository) FindByPlatformAndUser(ctx context.Context, platform string, userID uint) (*model.AutoReplyRule, error) {
	var rule model.AutoReplyRule
	err := r.db.Where("platform = ? AND user_id = ?", platform, userID).Order("id DESC").First(&rule).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// GetByMerchantAndPlatform 查询该平台所有启用的自动回复规则
// (从 service/unified_message.go 迁出,五层架构合规:不调 db.GetDB())
func (r *AutoReplyRuleRepository) GetByMerchantAndPlatform(ctx context.Context, platform string) ([]*model.AutoReplyRule, error) {
	var rules []*model.AutoReplyRule

	err := r.db.Where("platform = ? AND is_active = ?", platform, true).
		Find(&rules).Error

	if err != nil {
		return nil, err
	}

	return rules, nil
}

// GetTodayCount 查询该规则今天已发送的回复数量
// (从 service/unified_message.go 迁出,五层架构合规)
func (r *AutoReplyRuleRepository) GetTodayCount(ctx context.Context, ruleID uint) (int, error) {
	// 获取今天的开始时间
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	var count int64
	err := r.db.Model(&model.UnifiedReply{}).
		Where("rule_id = ? AND created_at >= ?", ruleID, todayStart).
		Count(&count).Error

	if err != nil {
		return 0, err
	}

	return int(count), nil
}

// FindExistingByPlatformAndUser 查找平台与用户已存在的规则（用于 upsert）
func (r *AutoReplyRuleRepository) FindExistingByPlatformAndUser(ctx context.Context, platform string, userID uint) (*model.AutoReplyRule, error) {
	var existing model.AutoReplyRule
	err := r.db.Where("platform = ? AND user_id = ?", platform, userID).First(&existing).Error
	if err != nil {
		return nil, err
	}
	return &existing, nil
}

// Create 创建规则
func (r *AutoReplyRuleRepository) Create(ctx context.Context, rule *model.AutoReplyRule) error {
	return r.db.Create(rule).Error
}

// Save 保存规则
func (r *AutoReplyRuleRepository) Save(ctx context.Context, rule *model.AutoReplyRule) error {
	return r.db.Save(rule).Error
}

// GetByID 按 ID 获取规则
func (r *AutoReplyRuleRepository) GetByID(ctx context.Context, id uint) (*model.AutoReplyRule, error) {
	var rule model.AutoReplyRule
	if err := r.db.First(&rule, id).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

// DeleteByID 按 ID 删除规则
func (r *AutoReplyRuleRepository) DeleteByID(ctx context.Context, id uint) error {
	return r.db.Delete(&model.AutoReplyRule{}, id).Error
}

// ListByPlatformAndUserActive 列出平台与用户下启用的规则
func (r *AutoReplyRuleRepository) ListByPlatformAndUserActive(ctx context.Context, platform string, userID uint) ([]model.AutoReplyRule, error) {
	var rules []model.AutoReplyRule
	err := r.db.Where("platform = ? AND user_id = ? AND is_active = ?", platform, userID, true).Find(&rules).Error
	return rules, err
}

// ListWithFilters 按条件分页列表
func (r *AutoReplyRuleRepository) ListWithFilters(ctx context.Context, platform string, userID uint, isActive *bool, page, pageSize int) ([]model.AutoReplyRule, int64, error) {
	query := r.db.Model(&model.AutoReplyRule{})
	if platform != "" {
		query = query.Where("platform = ?", platform)
	}
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if isActive != nil {
		query = query.Where("is_active = ?", *isActive)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	var rules []model.AutoReplyRule
	err := query.Offset((page - 1) * pageSize).Limit(pageSize).Find(&rules).Error
	return rules, total, err
}

// FindTopDailyLimit 查询平台与用户下每日限额最大的规则
func (r *AutoReplyRuleRepository) FindTopDailyLimit(ctx context.Context, platform string, userID uint) (*model.AutoReplyRule, error) {
	var rule model.AutoReplyRule
	query := r.db.Model(&model.AutoReplyRule{})
	if platform != "" {
		query = query.Where("platform = ?", platform)
	}
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Order("daily_limit DESC").First(&rule).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

// CountByFilters 按条件统计规则数量
//
// 用于 controller 替代直接调用 GetDB().Model().Count() 的架构违规。
func (r *AutoReplyRuleRepository) CountByFilters(ctx context.Context, platform string, userID uint) (int64, error) {
	query := r.db.Model(&model.AutoReplyRule{})
	if platform != "" {
		query = query.Where("platform = ?", platform)
	}
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

// GetByMerchantAndPlatform 根据平台获取所有启用的自动回复规则
// (与上面带 ctx 版本合并,见第 110 行 GetByMerchantAndPlatform)

// GetTodayCount 统计指定规则在今日的触发次数（基于 AutoReplyLog）
// (与上面带 ctx 版本合并,见第 125 行 GetTodayCount)

// ============== AutoReplyLogRepository ==============

// AutoReplyLogRepository 自动回复日志仓库
type AutoReplyLogRepository struct {
	db *gorm.DB
}

// NewAutoReplyLogRepository 创建自动回复日志仓库实例
func NewAutoReplyLogRepository(db *gorm.DB) *AutoReplyLogRepository {
	return &AutoReplyLogRepository{db: db}
}

// Create 创建日志
func (r *AutoReplyLogRepository) Create(ctx context.Context, log *model.AutoReplyLog) error {
	return r.db.Create(log).Error
}

// ListRecentByPlatformAndUser 列出平台与用户最近的日志
func (r *AutoReplyLogRepository) ListRecentByPlatformAndUser(ctx context.Context, platform string, userID uint, page, pageSize int, cutoff time.Time) ([]model.AutoReplyLog, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	var total int64
	if err := r.db.Model(&model.AutoReplyLog{}).
		Where("platform = ? AND user_id = ? AND created_at >= ?", platform, userID, cutoff).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []model.AutoReplyLog
	err := r.db.Where("platform = ? AND user_id = ? AND created_at >= ?", platform, userID, cutoff).
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&logs).Error
	return logs, total, err
}

// CountByUserAndRuleSince 自指定时间起按用户与规则统计日志数
func (r *AutoReplyLogRepository) CountByUserAndRuleSince(ctx context.Context, userID, ruleID uint, since time.Time) (int64, error) {
	var count int64
	err := r.db.Model(&model.AutoReplyLog{}).
		Where("user_id = ? AND rule_id = ? AND created_at >= ?", userID, ruleID, since).
		Count(&count).Error
	return count, err
}

// DeleteSinceByFilters 按时间及可选过滤条件删除日志（用于重置每日限额）
func (r *AutoReplyLogRepository) DeleteSinceByFilters(ctx context.Context, since time.Time, platform string, userID, accountID uint) error {
	query := r.db.Model(&model.AutoReplyLog{}).Where("created_at >= ?", since)
	if platform != "" {
		query = query.Where("platform = ?", platform)
	}
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if accountID > 0 {
		query = query.Where("account_id = ?", accountID)
	}
	return query.Delete(nil).Error
}

// CountSinceByFilters 按时间及可选过滤条件统计日志数
func (r *AutoReplyLogRepository) CountSinceByFilters(ctx context.Context, since time.Time, platform string, userID, accountID uint) (int64, error) {
	var count int64
	query := r.db.Model(&model.AutoReplyLog{}).Where("created_at >= ?", since)
	if platform != "" {
		query = query.Where("platform = ?", platform)
	}
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if accountID > 0 {
		query = query.Where("account_id = ?", accountID)
	}
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountActiveAccountsSince 统计自指定时间起不同 account_id 的活跃数量（status=success）
func (r *AutoReplyLogRepository) CountActiveAccountsSince(ctx context.Context, since time.Time, platform string, userID uint) (int64, error) {
	var count int64
	query := r.db.Model(&model.AutoReplyLog{}).Where("created_at >= ?", since).Where("status = ?", "success")
	if platform != "" {
		query = query.Where("platform = ?", platform)
	}
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Distinct("account_id").Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountPendingByFilters 统计待处理（status=pending）日志数
func (r *AutoReplyLogRepository) CountPendingByFilters(ctx context.Context, platform string, userID uint) (int64, error) {
	var count int64
	query := r.db.Model(&model.AutoReplyLog{}).Where("status = ?", "pending")
	if platform != "" {
		query = query.Where("platform = ?", platform)
	}
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountByFilters 按条件统计日志数量
//
// 用于 controller 替代直接调用 GetDB().Model().Count() 的架构违规。
func (r *AutoReplyLogRepository) CountByFilters(ctx context.Context, platform string, userID uint) (int64, error) {
	var count int64
	query := r.db.Model(&model.AutoReplyLog{})
	if platform != "" {
		query = query.Where("platform = ?", platform)
	}
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountByFiltersAndDate 按条件+指定日期统计日志数量
//
// 用于今日日志统计，替代 controller 直接调用 GetDB().Model().Where("DATE(created_at)...").Count()。
func (r *AutoReplyLogRepository) CountByFiltersAndDate(ctx context.Context, platform string, userID uint, date string) (int64, error) {
	var count int64
	query := r.db.Model(&model.AutoReplyLog{}).Where("DATE(created_at) = ?", date)
	if platform != "" {
		query = query.Where("platform = ?", platform)
	}
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
