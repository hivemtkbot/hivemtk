package repository

import (
	"context"
	"fmt"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
	"time"

	"gorm.io/gorm"
)

// WeComAccountRepository 企业微信账号仓库
type WeComAccountRepository struct {
	db *gorm.DB
}

// NewWeComAccountRepository 创建企业微信账号仓库实例
func NewWeComAccountRepository() *WeComAccountRepository {
	return &WeComAccountRepository{
		db: _db.GetDB(),
	}
}

// SetDB 注入 db（用于测试）
func (r *WeComAccountRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// Create 创建账号
func (r *WeComAccountRepository) Create(ctx context.Context, account *model.WeComAccount) error {
	return r.db.Create(account).Error
}

// GetByID 根据 ID 获取账号
func (r *WeComAccountRepository) GetByID(ctx context.Context, id uint) (*model.WeComAccount, error) {
	var account model.WeComAccount
	err := r.db.First(&account, id).Error
	return &account, err
}

func (r *WeComAccountRepository) GetByMerchant(ctx context.Context) ([]*model.WeComAccount, error) {
	var accounts []*model.WeComAccount
	err := r.db.Find(&accounts).Error
	return accounts, err
}

// Update 更新账号
func (r *WeComAccountRepository) Update(ctx context.Context, account *model.WeComAccount) error {
	return r.db.Save(account).Error
}

// UpdateToken 更新访问令牌
func (r *WeComAccountRepository) UpdateToken(ctx context.Context, id uint, token string, expires time.Time) error {
	return r.db.Model(&model.WeComAccount{}).Where("id = ?", id).
		Updates(map[string]any{
			"access_token":  token,
			"token_expires": expires,
		}).Error
}

// Delete 删除账号
func (r *WeComAccountRepository) Delete(ctx context.Context, id uint) error {
	return r.db.Delete(&model.WeComAccount{}, id).Error
}

// UpdateSyncTime 更新同步时间
func (r *WeComAccountRepository) UpdateSyncTime(ctx context.Context, id uint) error {
	now := make(map[string]any)
	now["last_sync_at"] = gorm.Expr("NOW()")
	return r.db.Model(&model.WeComAccount{}).Where("id = ?", id).Updates(now).Error
}

// WeComCustomerRepository 企业微信客户仓库
type WeComCustomerRepository struct {
	db *gorm.DB
}

// NewWeComCustomerRepository 创建企业微信客户仓库实例
func NewWeComCustomerRepository() *WeComCustomerRepository {
	return &WeComCustomerRepository{
		db: _db.GetDB(),
	}
}

// Create 创建客户
func (r *WeComCustomerRepository) Create(ctx context.Context, customer *model.WeComCustomer) error {
	return r.db.Create(customer).Error
}

// GetByID 根据 ID 获取客户
func (r *WeComCustomerRepository) GetByID(ctx context.Context, id uint) (*model.WeComCustomer, error) {
	var customer model.WeComCustomer
	err := r.db.First(&customer, id).Error
	return &customer, err
}

// GetByExternalUserID 根据外部用户 ID 获取客户（独立部署：单租户）
func (r *WeComCustomerRepository) GetByExternalUserID(ctx context.Context, externalUserID string) (*model.WeComCustomer, error) {
	var customer model.WeComCustomer
	err := r.db.Where("external_user_id = ?", externalUserID).First(&customer).Error
	return &customer, err
}

func (r *WeComCustomerRepository) GetByMerchant(ctx context.Context, page, pageSize int) ([]*model.WeComCustomer, int64, error) {
	var customers []*model.WeComCustomer
	var total int64

	r.db.Model(&model.WeComCustomer{}).Count(&total)
	err := r.db.
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&customers).Error

	return customers, total, err
}

// Update 更新客户
func (r *WeComCustomerRepository) Update(ctx context.Context, customer *model.WeComCustomer) error {
	return r.db.Save(customer).Error
}

// Delete 删除客户
func (r *WeComCustomerRepository) Delete(ctx context.Context, id uint) error {
	return r.db.Delete(&model.WeComCustomer{}, id).Error
}

// GetByEmployeeID 根据员工 ID 获取客户列表（独立部署：单租户）
func (r *WeComCustomerRepository) GetByEmployeeID(ctx context.Context, employeeID string, page, pageSize int) ([]*model.WeComCustomer, int64, error) {
	var customers []*model.WeComCustomer
	var total int64

	r.db.Model(&model.WeComCustomer{}).Where("employee_id = ?", employeeID).Count(&total)
	err := r.db.Where("employee_id = ?", employeeID).
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&customers).Error

	return customers, total, err
}

// WeComGroupRepository 企业微信客户群仓库
type WeComGroupRepository struct {
	db *gorm.DB
}

// NewWeComGroupRepository 创建企业微信客户群仓库实例
func NewWeComGroupRepository() *WeComGroupRepository {
	return &WeComGroupRepository{
		db: _db.GetDB(),
	}
}

// Create 创建群
func (r *WeComGroupRepository) Create(ctx context.Context, group *model.WeComGroup) error {
	return r.db.Create(group).Error
}

// GetByID 根据 ID 获取群
func (r *WeComGroupRepository) GetByID(ctx context.Context, id uint) (*model.WeComGroup, error) {
	var group model.WeComGroup
	err := r.db.First(&group, id).Error
	return &group, err
}

// GetByChatID 根据群 ID 获取群（独立部署：单租户）
func (r *WeComGroupRepository) GetByChatID(ctx context.Context, chatID string) (*model.WeComGroup, error) {
	var group model.WeComGroup
	err := r.db.Where("chat_id = ?", chatID).First(&group).Error
	return &group, err
}

func (r *WeComGroupRepository) GetByMerchant(ctx context.Context, page, pageSize int) ([]*model.WeComGroup, int64, error) {
	var groups []*model.WeComGroup
	var total int64

	r.db.Model(&model.WeComGroup{}).Count(&total)
	err := r.db.
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&groups).Error

	return groups, total, err
}

// Update 更新群
func (r *WeComGroupRepository) Update(ctx context.Context, group *model.WeComGroup) error {
	return r.db.Save(group).Error
}

// Delete 删除群
func (r *WeComGroupRepository) Delete(ctx context.Context, id uint) error {
	return r.db.Delete(&model.WeComGroup{}, id).Error
}

// UpdateMemberCount 更新成员数量
func (r *WeComGroupRepository) UpdateMemberCount(ctx context.Context, chatID string, count int) error {
	return r.db.Model(&model.WeComGroup{}).Where("chat_id = ?", chatID).Update("member_count", count).Error
}

// WeComGroupMemberRepository 企业微信客户群成员仓库
type WeComGroupMemberRepository struct {
	db *gorm.DB
}

// NewWeComGroupMemberRepository 创建企业微信客户群成员仓库实例
func NewWeComGroupMemberRepository() *WeComGroupMemberRepository {
	return &WeComGroupMemberRepository{
		db: _db.GetDB(),
	}
}

// Create 创建群成员
func (r *WeComGroupMemberRepository) Create(ctx context.Context, member *model.WeComGroupMember) error {
	return r.db.Create(member).Error
}

// GetByGroupID 根据群 ID 获取成员列表
func (r *WeComGroupMemberRepository) GetByGroupID(ctx context.Context, groupID uint, page, pageSize int) ([]*model.WeComGroupMember, int64, error) {
	var members []*model.WeComGroupMember
	var total int64

	r.db.Model(&model.WeComGroupMember{}).Where("group_id = ?", groupID).Count(&total)
	err := r.db.Where("group_id = ?", groupID).
		Order("join_time DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&members).Error

	return members, total, err
}

// Delete 删除群成员
func (r *WeComGroupMemberRepository) Delete(ctx context.Context, id uint) error {
	return r.db.Delete(&model.WeComGroupMember{}, id).Error
}

// DeleteByGroupID 删除群下所有成员
func (r *WeComGroupMemberRepository) DeleteByGroupID(ctx context.Context, groupID uint) error {
	return r.db.Where("group_id = ?", groupID).Delete(&model.WeComGroupMember{}).Error
}

// WeComMessageRepository 企业微信消息仓库
type WeComMessageRepository struct {
	db *gorm.DB
}

// NewWeComMessageRepository 创建企业微信消息仓库实例
func NewWeComMessageRepository() *WeComMessageRepository {
	return &WeComMessageRepository{
		db: _db.GetDB(),
	}
}

// Create 创建消息
func (r *WeComMessageRepository) Create(ctx context.Context, message *model.WeComMessage) error {
	return r.db.Create(message).Error
}

// GetByID 根据 ID 获取消息
func (r *WeComMessageRepository) GetByID(ctx context.Context, id uint) (*model.WeComMessage, error) {
	var message model.WeComMessage
	err := r.db.First(&message, id).Error
	return &message, err
}

func (r *WeComMessageRepository) GetByMerchant(ctx context.Context, page, pageSize int) ([]*model.WeComMessage, int64, error) {
	var messages []*model.WeComMessage
	var total int64

	r.db.Model(&model.WeComMessage{}).Count(&total)
	err := r.db.
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&messages).Error

	return messages, total, err
}

// UpdateStatus 更新消息状态
func (r *WeComMessageRepository) UpdateStatus(ctx context.Context, id uint, status int, sendTime time.Time, errorMsg string) error {
	updates := map[string]any{
		"status": status,
	}
	if sendTime.IsZero() {
		updates["send_time"] = gorm.Expr("NOW()")
	}
	if errorMsg != "" {
		updates["error_msg"] = errorMsg
	}
	return r.db.Model(&model.WeComMessage{}).Where("id = ?", id).Updates(updates).Error
}

// WeComTagRepository 企业微信标签仓库
type WeComTagRepository struct {
	db *gorm.DB
}

// NewWeComTagRepository 创建企业微信标签仓库实例
func NewWeComTagRepository(db *gorm.DB) *WeComTagRepository {
	return &WeComTagRepository{
		db: db,
	}
}

// Create 创建标签
func (r *WeComTagRepository) Create(ctx context.Context, tag *model.WeComTag) error {
	return r.db.Create(tag).Error
}

func (r *WeComTagRepository) GetByMerchant(ctx context.Context) ([]*model.WeComTag, error) {
	var tags []*model.WeComTag
	err := r.db.Find(&tags).Error
	return tags, err
}

// Update 更新标签
func (r *WeComTagRepository) Update(ctx context.Context, tag *model.WeComTag) error {
	return r.db.Save(tag).Error
}

// Delete 删除标签
func (r *WeComTagRepository) Delete(ctx context.Context, id uint) error {
	return r.db.Delete(&model.WeComTag{}, id).Error
}

// ============================================================================
// WeComAccountRepository 扩展方法（服务于 WeComAccountHealthService）
// ============================================================================

// FindByRiskLevels 按风险等级列表筛选账号（私域独立部署：无 merchant_id）
func (r *WeComAccountRepository) FindByRiskLevels(ctx context.Context, riskLevels []string) ([]model.WeComAccount, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	if len(riskLevels) == 0 {
		return nil, nil
	}
	var accounts []model.WeComAccount
	err := r.db.WithContext(ctx).Where("risk_level IN ?", riskLevels).Find(&accounts).Error
	return accounts, err
}

// FindHealthyAccounts 查询非排除登录状态且风险等级在指定范围内的账号
// excludeLoginStates: 需要排除的登录状态（如 banned/offline）
func (r *WeComAccountRepository) FindHealthyAccounts(ctx context.Context, riskLevels []string, excludeLoginStates []string) ([]model.WeComAccount, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	if len(riskLevels) == 0 {
		return nil, nil
	}
	q := r.db.WithContext(ctx).Model(&model.WeComAccount{}).Where("risk_level IN ?", riskLevels)
	if len(excludeLoginStates) > 0 {
		q = q.Where("login_state NOT IN ?", excludeLoginStates)
	}
	var accounts []model.WeComAccount
	err := q.Find(&accounts).Error
	return accounts, err
}

// ============================================================================
// WeComAccountHealthRepository 企业微信账号健康度仓库
// 五层架构归属: L3 仓库层
// ============================================================================

// WeComAccountHealthRepository 企业微信账号健康度仓库
type WeComAccountHealthRepository struct {
	db *gorm.DB
}

// NewWeComAccountHealthRepository 创建企业微信账号健康度仓库实例
func NewWeComAccountHealthRepository() *WeComAccountHealthRepository {
	return &WeComAccountHealthRepository{
		db: _db.GetDB(),
	}
}

// SetDB 注入 db（用于测试）
//
// 五层架构 §三.5 + §七：仓库方法必须首参为 ctx context.Context。
func (r *WeComAccountHealthRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// Create 创建健康度记录
func (r *WeComAccountHealthRepository) Create(ctx context.Context, rec *model.WeComAccountHealth) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("health repo is nil")
	}
	return r.db.WithContext(ctx).Create(rec).Error
}

// GetLatestByAccountID 获取账号最新健康度记录
func (r *WeComAccountHealthRepository) GetLatestByAccountID(ctx context.Context, accountID uint) (*model.WeComAccountHealth, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var rec model.WeComAccountHealth
	err := r.db.WithContext(ctx).
		Where("account_id = ?", accountID).
		Order("reported_at DESC, id DESC").
		First(&rec).Error
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// ListByAccountIDPaged 分页列出账号健康度历史（按 reported_at DESC）
func (r *WeComAccountHealthRepository) ListByAccountIDPaged(ctx context.Context, accountID uint, page, pageSize int) ([]model.WeComAccountHealth, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, nil
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	var total int64
	q := r.db.WithContext(ctx).Model(&model.WeComAccountHealth{}).Where("account_id = ?", accountID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.WeComAccountHealth
	err := q.Order("reported_at DESC, id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&items).Error
	return items, total, err
}

// UpdateFields 按 ID 更新指定字段（map 形式，支持 gorm.Expr）
// 服务于 WeComAccountHealthService.IncrementSentCount / syncAccountState 等场景
func (r *WeComAccountRepository) UpdateFields(ctx context.Context, id uint, fields map[string]any) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("wecom account repo is nil")
	}
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.WeComAccount{}).
		Where("id = ?", id).
		Updates(fields).Error
}

// UpdateAllFields 批量更新所有账号的指定字段（无 WHERE 条件）
// 服务于 WeComAccountHealthService.ResetDailyQuota（每日凌晨重置所有账号配额）
// 返回受影响行数
func (r *WeComAccountRepository) UpdateAllFields(ctx context.Context, fields map[string]any) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("wecom account repo is nil")
	}
	if len(fields) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Model(&model.WeComAccount{}).
		Where("1 = 1").
		Updates(fields)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// ListAllOrderByIDDesc 列出所有账号（按 ID DESC 排序）
// 服务于 WeComAccountHealthService.GetHealthSummary / WeComIntegrationService.ListAccountsWithHealth
func (r *WeComAccountRepository) ListAllOrderByIDDesc(ctx context.Context) ([]model.WeComAccount, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var accounts []model.WeComAccount
	err := r.db.WithContext(ctx).Order("id DESC").Find(&accounts).Error
	return accounts, err
}
