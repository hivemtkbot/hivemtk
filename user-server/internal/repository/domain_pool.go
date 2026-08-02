package repository

import (
	"context"
	"errors"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
	"time"

	"gorm.io/gorm"
)

// DomainPoolRepository 域名池仓储接口
type DomainPoolRepository interface {
	Create(ctx context.Context, domainPool *model.DomainPool) error
	Update(ctx context.Context, domainPool *model.DomainPool) error
	Delete(ctx context.Context, id int) error
	GetByID(ctx context.Context, id int) (*model.DomainPool, error)
	GetByDomain(ctx context.Context, domain string) (*model.DomainPool, error)
	List(ctx context.Context, page, pageSize int, domain string, status int) ([]*model.DomainPool, int64, error)
	UpdateStatus(ctx context.Context, id, status int) error
	UpdateLastCheck(ctx context.Context, id int, lastCheck time.Time) error

	// G 域 ：健康度自动切换
	UpdateHealth(ctx context.Context, id int, score, consecutiveFailures int, dnsOK bool, dnsErr string, httpStatus, latencyMs int, onBlacklist bool, blacklistAt time.Time, blacklistNote string) error
	UpdateActive(ctx context.Context, id int, isActive bool, switchedAt *time.Time, switchedFromID int) error
	ListActive(ctx context.Context) ([]*model.DomainPool, error)
	// ListAvailable 返回健康度评分 >= minScore 且未在黑名单的可用域名
	ListAvailable(ctx context.Context, minScore int) ([]*model.DomainPool, error)
	// DeactivateAll 取消所有当前活跃域名
	DeactivateAll(ctx context.Context) error
}

// domainPoolRepository 域名池仓储实现
type domainPoolRepository struct {
	db *gorm.DB
}

// NewDomainPoolRepository 创建域名池仓储实例
func NewDomainPoolRepository(db *gorm.DB) DomainPoolRepository {
	return &domainPoolRepository{db: db}
}

// NewDomainPoolRepositoryWithDB 测试用：显式指定 db
func NewDomainPoolRepositoryWithDB(db *gorm.DB) DomainPoolRepository {
	return &domainPoolRepository{db: db}
}

// dbOrDefault 当未注入 db 时回退到全局
func (r *domainPoolRepository) dbOrDefault(ctx context.Context) *gorm.DB {
	if r.db != nil {
		return r.db
	}
	return _db.GetDB()
}

// Create 创建域名池记录
func (r *domainPoolRepository) Create(ctx context.Context, domainPool *model.DomainPool) error {
	return r.dbOrDefault(ctx).Create(domainPool).Error
}

// Update 更新域名池记录
func (r *domainPoolRepository) Update(ctx context.Context, domainPool *model.DomainPool) error {
	return r.dbOrDefault(ctx).Save(domainPool).Error
}

// Delete 删除域名池记录
func (r *domainPoolRepository) Delete(ctx context.Context, id int) error {
	return r.dbOrDefault(ctx).Delete(&model.DomainPool{}, id).Error
}

// GetByID 根据ID获取域名池记录
func (r *domainPoolRepository) GetByID(ctx context.Context, id int) (*model.DomainPool, error) {
	var domainPool model.DomainPool
	err := r.dbOrDefault(ctx).First(&domainPool, id).Error
	if err != nil {
		return nil, err
	}
	return &domainPool, nil
}

// GetByDomain 根据域名获取域名池记录
func (r *domainPoolRepository) GetByDomain(ctx context.Context, domain string) (*model.DomainPool, error) {
	var domainPool model.DomainPool
	err := r.dbOrDefault(ctx).Where("domain = ?", domain).First(&domainPool).Error
	if err != nil {
		return nil, err
	}
	return &domainPool, nil
}

// List 获取域名池列表
func (r *domainPoolRepository) List(ctx context.Context, page, pageSize int, domain string, status int) ([]*model.DomainPool, int64, error) {
	var domainPools []*model.DomainPool
	var total int64

	query := r.dbOrDefault(ctx).Model(&model.DomainPool{})

	// 添加搜索条件
	if domain != "" {
		query = query.Where("domain LIKE ?", "%"+domain+"%")
	}
	if status > 0 {
		query = query.Where("status = ?", status)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&domainPools).Error; err != nil {
		return nil, 0, err
	}

	return domainPools, total, nil
}

// UpdateStatus 更新域名池状态
func (r *domainPoolRepository) UpdateStatus(ctx context.Context, id, status int) error {
	return r.dbOrDefault(ctx).Model(&model.DomainPool{}).Where("id = ?", id).Update("status", status).Error
}

// UpdateLastCheck 更新最后检查时间
func (r *domainPoolRepository) UpdateLastCheck(ctx context.Context, id int, lastCheck time.Time) error {
	return r.dbOrDefault(ctx).Model(&model.DomainPool{}).Where("id = ?", id).Update("last_check", lastCheck).Error
}

// UpdateHealth G 域 ：写入健康度评分
func (r *domainPoolRepository) UpdateHealth(
	ctx context.Context,
	id int,
	score, consecutiveFailures int,
	dnsOK bool,
	dnsErr string,
	httpStatus, latencyMs int,
	onBlacklist bool,
	blacklistAt time.Time,
	blacklistNote string,
) error {
	if id <= 0 {
		return errors.New("invalid domain id")
	}
	updates := map[string]any{
		"health_score":         score,
		"consecutive_failures": consecutiveFailures,
		"dns_resolved":         dnsOK,
		"dns_error":            dnsErr,
		"last_http_status":     httpStatus,
		"last_latency_ms":      latencyMs,
		"on_blacklist":         onBlacklist,
		"blacklist_note":       blacklistNote,
		"last_check":           time.Now(),
	}
	if onBlacklist && blacklistAt.IsZero() {
		updates["blacklist_at"] = time.Now()
	} else if !onBlacklist {
		updates["blacklist_at"] = time.Time{}
	}
	return r.dbOrDefault(ctx).Model(&model.DomainPool{}).Where("id = ?", id).Updates(updates).Error
}

// UpdateActive G 域 ：写入活跃状态与切换时间
func (r *domainPoolRepository) UpdateActive(ctx context.Context, id int, isActive bool, switchedAt *time.Time, switchedFromID int) error {
	updates := map[string]any{
		"is_active":        isActive,
		"switched_at":      switchedAt,
		"switched_from_id": switchedFromID,
	}
	return r.dbOrDefault(ctx).Model(&model.DomainPool{}).Where("id = ?", id).Updates(updates).Error
}

// ListActive 返回所有当前标记为活跃的域名
func (r *domainPoolRepository) ListActive(ctx context.Context) ([]*model.DomainPool, error) {
	var rows []*model.DomainPool
	err := r.dbOrDefault(ctx).Where("is_active = ?", true).Order("switched_at DESC NULLS LAST").Find(&rows).Error
	return rows, err
}

// ListAvailable 返回评分 >= minScore、未在黑名单、状态为正常的可用域名
func (r *domainPoolRepository) ListAvailable(ctx context.Context, minScore int) ([]*model.DomainPool, error) {
	var rows []*model.DomainPool
	err := r.dbOrDefault(ctx).
		Where("health_score >= ?", minScore).
		Where("on_blacklist = ?", false).
		Where("status = ?", 1).
		Order("health_score DESC, last_check DESC").
		Find(&rows).Error
	return rows, err
}

// DeactivateAll 将所有域名标记为非活跃
func (r *domainPoolRepository) DeactivateAll(ctx context.Context) error {
	return r.dbOrDefault(ctx).Model(&model.DomainPool{}).Where("is_active = ?", true).Update("is_active", false).Error
}

// ============== 健康度日志 ==============

// DomainHealthLogRepository 健康度日志仓储
type DomainHealthLogRepository struct {
	db *gorm.DB
}

// NewDomainHealthLogRepository 创建健康度日志仓储
func NewDomainHealthLogRepository(db *gorm.DB) *DomainHealthLogRepository {
	if db == nil {
		db = _db.GetDB()
	}
	return &DomainHealthLogRepository{db: db}
}

// Create 创建一条健康度日志
func (r *DomainHealthLogRepository) Create(ctx context.Context, log *model.DomainHealthLog) error {
	if log.CheckedAt.IsZero() {
		log.CheckedAt = time.Now()
	}
	return r.db.Create(log).Error
}

// ListByDomain 查询指定域名的最近 N 条日志
func (r *DomainHealthLogRepository) ListByDomain(ctx context.Context, domainID int, limit int) ([]*model.DomainHealthLog, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []*model.DomainHealthLog
	err := r.db.Where("domain_id = ?", domainID).Order("checked_at DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

// ============== 黑名单 ==============

// DomainBlacklistRepository 黑名单仓储
type DomainBlacklistRepository struct {
	db *gorm.DB
}

// NewDomainBlacklistRepository 创建黑名单仓储
func NewDomainBlacklistRepository(db *gorm.DB) *DomainBlacklistRepository {
	if db == nil {
		db = _db.GetDB()
	}
	return &DomainBlacklistRepository{db: db}
}

// IsBlacklisted 判定域名是否在黑名单中（Active + 未过期）
func (r *DomainBlacklistRepository) IsBlacklisted(ctx context.Context, domain string) (bool, *model.DomainBlacklist, error) {
	if domain == "" {
		return false, nil, nil
	}
	var row model.DomainBlacklist
	err := r.db.Where("domain = ? AND active = ?", domain, true).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil, nil
		}
		return false, nil, err
	}
	// 过期判断
	if row.ExpiresAt != nil && row.ExpiresAt.Before(time.Now()) {
		return false, &row, nil
	}
	return true, &row, nil
}

// Add 添加黑名单（幂等：已存在则刷新 reason）
func (r *DomainBlacklistRepository) Add(ctx context.Context, domain, platform, reason, source string, expiresAt *time.Time) error {
	if domain == "" {
		return errors.New("domain is required")
	}
	if platform == "" {
		platform = "all"
	}
	if source == "" {
		source = "system"
	}
	// upsert by domain
	var existing model.DomainBlacklist
	err := r.db.Where("domain = ?", domain).First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			row := &model.DomainBlacklist{
				Domain:    domain,
				Platform:  platform,
				Reason:    reason,
				Source:    source,
				ExpiresAt: expiresAt,
				Active:    true,
			}
			return r.db.Create(row).Error
		}
		return err
	}
	updates := map[string]any{
		"platform":   platform,
		"reason":     reason,
		"source":     source,
		"expires_at": expiresAt,
		"active":     true,
	}
	return r.db.Model(&model.DomainBlacklist{}).Where("id = ?", existing.ID).Updates(updates).Error
}

// Remove 移除黑名单
func (r *DomainBlacklistRepository) Remove(ctx context.Context, domain string) error {
	return r.db.Model(&model.DomainBlacklist{}).Where("domain = ?", domain).Update("active", false).Error
}

// List 分页查询
func (r *DomainBlacklistRepository) List(ctx context.Context, page, pageSize int) ([]*model.DomainBlacklist, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	var rows []*model.DomainBlacklist
	var total int64
	if err := r.db.Model(&model.DomainBlacklist{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := r.db.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
