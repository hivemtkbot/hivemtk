package repository

import (
	"context"
	"errors"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
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

	UpdateHealth(ctx context.Context, id int, score, consecutiveFailures int, dnsOK bool, dnsErr string, httpStatus, latencyMs int, onBlacklist bool, blacklistAt time.Time, blacklistNote string) error
	UpdateActive(ctx context.Context, id int, isActive bool, switchedAt *time.Time, switchedFromID int) error
	ListActive(ctx context.Context) ([]*model.DomainPool, error)
	ListAvailable(ctx context.Context, minScore int) ([]*model.DomainPool, error)
	DeactivateAll(ctx context.Context) error
}

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

func (r *domainPoolRepository) dbOrDefault(ctx context.Context) *gorm.DB {
	if r.db != nil {
		return r.db
	}
	return _db.GetDB()
}

func (r *domainPoolRepository) Create(ctx context.Context, domainPool *model.DomainPool) error {
	return r.dbOrDefault(ctx).Create(domainPool).Error
}

func (r *domainPoolRepository) Update(ctx context.Context, domainPool *model.DomainPool) error {
	return r.dbOrDefault(ctx).Save(domainPool).Error
}

func (r *domainPoolRepository) Delete(ctx context.Context, id int) error {
	return r.dbOrDefault(ctx).Delete(&model.DomainPool{}, id).Error
}

func (r *domainPoolRepository) GetByID(ctx context.Context, id int) (*model.DomainPool, error) {
	var domainPool model.DomainPool
	err := r.dbOrDefault(ctx).First(&domainPool, id).Error
	if err != nil {
		return nil, err
	}
	return &domainPool, nil
}

func (r *domainPoolRepository) GetByDomain(ctx context.Context, domain string) (*model.DomainPool, error) {
	var domainPool model.DomainPool
	err := r.dbOrDefault(ctx).Where("domain = ?", domain).First(&domainPool).Error
	if err != nil {
		return nil, err
	}
	return &domainPool, nil
}

func (r *domainPoolRepository) List(ctx context.Context, page, pageSize int, domain string, status int) ([]*model.DomainPool, int64, error) {
	var domainPools []*model.DomainPool
	var total int64

	query := r.dbOrDefault(ctx).Model(&model.DomainPool{})

	if domain != "" {
		query = query.Where("domain LIKE ?", "%"+domain+"%")
	}
	if status > 0 {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&domainPools).Error; err != nil {
		return nil, 0, err
	}

	return domainPools, total, nil
}

func (r *domainPoolRepository) UpdateStatus(ctx context.Context, id, status int) error {
	return r.dbOrDefault(ctx).Model(&model.DomainPool{}).Where("id = ?", id).Update("status", status).Error
}

func (r *domainPoolRepository) UpdateLastCheck(ctx context.Context, id int, lastCheck time.Time) error {
	return r.dbOrDefault(ctx).Model(&model.DomainPool{}).Where("id = ?", id).Update("last_check", lastCheck).Error
}

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

func (r *domainPoolRepository) UpdateActive(ctx context.Context, id int, isActive bool, switchedAt *time.Time, switchedFromID int) error {
	updates := map[string]any{
		"is_active":        isActive,
		"switched_at":      switchedAt,
		"switched_from_id": switchedFromID,
	}
	return r.dbOrDefault(ctx).Model(&model.DomainPool{}).Where("id = ?", id).Updates(updates).Error
}

func (r *domainPoolRepository) ListActive(ctx context.Context) ([]*model.DomainPool, error) {
	var rows []*model.DomainPool
	err := r.dbOrDefault(ctx).Where("is_active = ?", true).Order("switched_at DESC NULLS LAST").Find(&rows).Error
	return rows, err
}

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

func (r *domainPoolRepository) DeactivateAll(ctx context.Context) error {
	return r.dbOrDefault(ctx).Model(&model.DomainPool{}).Where("is_active = ?", true).Update("is_active", false).Error
}

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
