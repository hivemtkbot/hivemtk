package repository

import (
	"marketing/internal/model"
	"time"

	"gorm.io/gorm"
)

type CardAccessRepository interface {
	Create(access *model.CardAccess) error
	GetByCardID(cardID uint, page, pageSize int) ([]*model.CardAccess, int64, error)
	CountAccess(cardID uint, cardType string, startDate, endDate time.Time) (int, error)
	CountDistinctIP(cardID uint, cardType string, startDate, endDate time.Time) (int, error)
	HasAccessToday(cardID uint, ip string) (bool, error)
}

type cardAccessRepository struct {
	db *gorm.DB
}

func NewCardAccessRepository(db *gorm.DB) CardAccessRepository {
	return &cardAccessRepository{db: db}
}

func (r *cardAccessRepository) Create(access *model.CardAccess) error {
	return r.db.Create(access).Error
}

func (r *cardAccessRepository) GetByCardID(cardID uint, page, pageSize int) ([]*model.CardAccess, int64, error) {
	var accesses []*model.CardAccess
	var total int64

	err := r.db.Model(&model.CardAccess{}).Where("card_id = ?", cardID).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = r.db.Where("card_id = ?", cardID).Order("access_time DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&accesses).Error

	return accesses, total, err
}

func (r *cardAccessRepository) CountAccess(cardID uint, cardType string, startDate, endDate time.Time) (int, error) {
	var count int64
	query := r.db.Model(&model.CardAccess{}).Where("card_id = ?", cardID)

	if cardType != "" {
		query = query.Where("card_type = ?", cardType)
	}

	if !startDate.IsZero() {
		query = query.Where("access_time >= ?", startDate)
	}

	if !endDate.IsZero() {
		query = query.Where("access_time <= ?", endDate)
	}

	err := query.Count(&count).Error
	return int(count), err
}

func (r *cardAccessRepository) CountDistinctIP(cardID uint, cardType string, startDate, endDate time.Time) (int, error) {
	var count int64
	query := r.db.Model(&model.CardAccess{}).Where("card_id = ?", cardID)

	if cardType != "" {
		query = query.Where("card_type = ?", cardType)
	}

	if !startDate.IsZero() {
		query = query.Where("access_time >= ?", startDate)
	}

	if !endDate.IsZero() {
		query = query.Where("access_time <= ?", endDate)
	}

	err := query.Distinct("ip_address").Count(&count).Error
	return int(count), err
}

func (r *cardAccessRepository) HasAccessToday(cardID uint, ip string) (bool, error) {
	today := time.Now().Format("2006-01-02")
	tomorrow, _ := time.Parse("2006-01-02", today)
	tomorrow = tomorrow.Add(24 * time.Hour)

	var count int64
	err := r.db.Model(&model.CardAccess{}).
		Where("card_id = ? AND ip_address = ? AND access_time >= ? AND access_time < ?",
			cardID, ip, today+" 00:00:00", tomorrow.Format("2006-01-02 15:04:05")).
		Count(&count).Error

	return count > 0, err
}

type DailyCardUVStatsRepository interface {
	Create(stats *model.DailyCardUVStats) error
	Update(stats *model.DailyCardUVStats) error
	GetByCardID(cardID uint, cardType string) ([]*model.DailyCardUVStats, error)
	GetByCardIDAndDate(cardID uint, cardType string, date string) (*model.DailyCardUVStats, error)
}

type dailyCardUVStatsRepository struct {
	db *gorm.DB
}

func NewDailyCardUVStatsRepository(db *gorm.DB) DailyCardUVStatsRepository {
	return &dailyCardUVStatsRepository{db: db}
}

func (r *dailyCardUVStatsRepository) Create(stats *model.DailyCardUVStats) error {
	return r.db.Create(stats).Error
}

func (r *dailyCardUVStatsRepository) Update(stats *model.DailyCardUVStats) error {
	return r.db.Save(stats).Error
}

func (r *dailyCardUVStatsRepository) GetByCardID(cardID uint, cardType string) ([]*model.DailyCardUVStats, error) {
	var stats []*model.DailyCardUVStats
	query := r.db.Where("card_id = ?", cardID)

	if cardType != "" {
		query = query.Where("card_type = ?", cardType)
	}

	err := query.Order("date DESC").Find(&stats).Error
	return stats, err
}

func (r *dailyCardUVStatsRepository) GetByCardIDAndDate(cardID uint, cardType string, date string) (*model.DailyCardUVStats, error) {
	var stats model.DailyCardUVStats
	err := r.db.Where("card_id = ? AND card_type = ? AND date = ?", cardID, cardType, date).
		First(&stats).Error

	if err == gorm.ErrRecordNotFound {
		return nil, err
	}

	return &stats, err
}
