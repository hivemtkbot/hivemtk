package repository

import (
	"gorm.io/gorm"
	"marketing/internal/model"
	"time"
)

// ShortLinkAccessRepository 短链访问统计仓储接口
type ShortLinkAccessRepository interface {
	Create(access *model.ShortLinkAccess) error
	GetByID(id uint) (*model.ShortLinkAccess, error)
	GetByShortLinkID(shortLinkID uint, page, pageSize int) ([]*model.ShortLinkAccess, int64, error)
	GetStatsByShortLinkID(shortLinkID uint, startDate, endDate time.Time) (*model.ShortLinkAccess, error)
	GetDailyStatsByShortLinkID(shortLinkID uint, startDate, endDate time.Time) ([]map[string]any, error)
	GetDeviceTypeStatsByShortLinkID(shortLinkID uint, startDate, endDate time.Time) ([]map[string]any, error)
	GetAllDailyStats(startDate, endDate time.Time) ([]map[string]any, error)
	GetAllDeviceTypeStats(startDate, endDate time.Time) ([]map[string]any, error)
	GetAllShortLinksBasicStats(startDate, endDate time.Time) ([]map[string]any, error)
}

// shortLinkAccessRepository 短链访问统计仓储实现
type shortLinkAccessRepository struct {
	db *gorm.DB
}

// NewShortLinkAccessRepository 创建短链访问统计仓储实例
func NewShortLinkAccessRepository(db *gorm.DB) ShortLinkAccessRepository {
	return &shortLinkAccessRepository{db: db}
}

// Create 创建短链访问记录
func (r *shortLinkAccessRepository) Create(access *model.ShortLinkAccess) error {
	return r.db.Create(access).Error
}

// GetByID 根据ID获取短链访问记录
func (r *shortLinkAccessRepository) GetByID(id uint) (*model.ShortLinkAccess, error) {
	var access model.ShortLinkAccess
	err := r.db.First(&access, id).Error
	if err != nil {
		return nil, err
	}
	return &access, nil
}

// GetByShortLinkID 根据短链ID获取访问记录
func (r *shortLinkAccessRepository) GetByShortLinkID(shortLinkID uint, page, pageSize int) ([]*model.ShortLinkAccess, int64, error) {
	var accesses []*model.ShortLinkAccess
	var total int64

	query := r.db.Model(&model.ShortLinkAccess{}).Where("short_link_id = ?", shortLinkID)

	// 获取总数
	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order("access_time DESC").Find(&accesses).Error
	if err != nil {
		return nil, 0, err
	}

	return accesses, total, nil
}

// GetStatsByShortLinkID 根据短链ID获取统计信息
func (r *shortLinkAccessRepository) GetStatsByShortLinkID(shortLinkID uint, startDate, endDate time.Time) (*model.ShortLinkAccess, error) {
	var stats struct {
		TotalCount int64 `json:"total_count"`
	}

	query := r.db.Model(&model.ShortLinkAccess{}).Where("short_link_id = ?", shortLinkID)

	if !startDate.IsZero() {
		query = query.Where("DATE(access_time) >= ?", startDate.Format("2006-01-02"))
	}

	if !endDate.IsZero() {
		query = query.Where("DATE(access_time) <= ?", endDate.Format("2006-01-02"))
	}

	err := query.Count(&stats.TotalCount).Error
	if err != nil {
		return nil, err
	}

	// 获取今日访问量
	today := time.Now().Format("2006-01-02")
	todayStart, _ := time.Parse("2006-01-02", today)

	var todayCount int64
	err = r.db.Model(&model.ShortLinkAccess{}).
		Where("short_link_id = ? AND DATE(access_time) >= ?", shortLinkID, todayStart.Format("2006-01-02")).
		Count(&todayCount).Error
	if err != nil {
		return nil, err
	}

	// 返回统计信息
	return &model.ShortLinkAccess{
		ShortLinkID: shortLinkID,
	}, nil
}

// GetDailyStatsByShortLinkID 根据短链ID获取每日访问统计
func (r *shortLinkAccessRepository) GetDailyStatsByShortLinkID(shortLinkID uint, startDate, endDate time.Time) ([]map[string]any, error) {
	query := r.db.Model(&model.ShortLinkAccess{}).
		Select("DATE(access_time) as date, COUNT(*) as count").
		Where("short_link_id = ?", shortLinkID)

	if !startDate.IsZero() {
		query = query.Where("DATE(access_time) >= ?", startDate.Format("2006-01-02"))
	}

	if !endDate.IsZero() {
		query = query.Where("DATE(access_time) <= ?", endDate.Format("2006-01-02"))
	}

	var results []map[string]any
	err := query.Group("DATE(access_time)").Order("date").Scan(&results).Error
	return results, err
}

// GetDeviceTypeStatsByShortLinkID 根据短链ID获取设备类型统计
func (r *shortLinkAccessRepository) GetDeviceTypeStatsByShortLinkID(shortLinkID uint, startDate, endDate time.Time) ([]map[string]any, error) {
	query := r.db.Model(&model.ShortLinkAccess{}).
		Select("device_type, COUNT(*) as count").
		Where("short_link_id = ?", shortLinkID)

	if !startDate.IsZero() {
		query = query.Where("DATE(access_time) >= ?", startDate.Format("2006-01-02"))
	}

	if !endDate.IsZero() {
		query = query.Where("DATE(access_time) <= ?", endDate.Format("2006-01-02"))
	}

	var results []map[string]any
	err := query.Group("device_type").Order("count DESC").Scan(&results).Error
	return results, err
}

// GetAllDailyStats 获取所有短链的每日访问统计
func (r *shortLinkAccessRepository) GetAllDailyStats(startDate, endDate time.Time) ([]map[string]any, error) {
	query := r.db.Model(&model.ShortLinkAccess{}).
		Select("DATE(access_time) as date, COUNT(*) as count")

	if !startDate.IsZero() {
		query = query.Where("DATE(access_time) >= ?", startDate.Format("2006-01-02"))
	}

	if !endDate.IsZero() {
		query = query.Where("DATE(access_time) <= ?", endDate.Format("2006-01-02"))
	}

	var results []map[string]any
	err := query.Group("DATE(access_time)").Order("date").Scan(&results).Error
	return results, err
}

// GetAllDeviceTypeStats 获取所有短链的设备类型统计
func (r *shortLinkAccessRepository) GetAllDeviceTypeStats(startDate, endDate time.Time) ([]map[string]any, error) {
	query := r.db.Model(&model.ShortLinkAccess{}).
		Select("device_type, COUNT(*) as count")

	if !startDate.IsZero() {
		query = query.Where("DATE(access_time) >= ?", startDate.Format("2006-01-02"))
	}

	if !endDate.IsZero() {
		query = query.Where("DATE(access_time) <= ?", endDate.Format("2006-01-02"))
	}

	var results []map[string]any
	err := query.Group("device_type").Order("count DESC").Scan(&results).Error
	return results, err
}

// GetAllShortLinksBasicStats 获取所有短链的基本统计
func (r *shortLinkAccessRepository) GetAllShortLinksBasicStats(startDate, endDate time.Time) ([]map[string]any, error) {
	query := r.db.Table("short_links sl").
		Select("sl.id, sl.short_code, sl.original_url AS title, COUNT(sla.id) as access_count").
		Joins("LEFT JOIN short_link_accesses sla ON sl.id = sla.short_link_id")

	if !startDate.IsZero() {
		query = query.Where("DATE(sla.access_time) >= ? OR sla.access_time IS NULL", startDate.Format("2006-01-02"))
	}

	if !endDate.IsZero() {
		query = query.Where("DATE(sla.access_time) <= ? OR sla.access_time IS NULL", endDate.Format("2006-01-02"))
	}

	var results []map[string]any
	err := query.Group("sl.id, sl.short_code, sl.original_url").Order("access_count DESC").Scan(&results).Error
	return results, err
}
