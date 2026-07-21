package service

import (
	"marketing/internal/model"
	"marketing/internal/pkg/utils"
	"marketing/internal/repository"
	"time"

	"gorm.io/gorm"
)

type CardAccessService interface {
	RecordAccess(cardID uint, cardType string, ip, ua, referer string) error
	GetCardUVStats(cardID uint, cardType string, startDate, endDate time.Time) (*CardStatsResponse, error)
	GetDailyUVStats(cardID uint, cardType string) ([]DailyStats, error)
	GetTodayUV(cardID uint, cardType string) (int, error)
}

type CardStatsResponse struct {
	CardID    uint      `json:"card_id"`
	CardType  string    `json:"card_type"`
	TotalUV   int       `json:"total_uv"`
	TotalPV   int       `json:"total_pv"`
	TodayUV   int       `json:"today_uv"`
	TodayPV   int       `json:"today_pv"`
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
}

type DailyStats struct {
	Date    string `json:"date"`
	UVCount int    `json:"uv_count"`
	PVCount int    `json:"pv_count"`
}

type cardAccessService struct {
	accessRepo  repository.CardAccessRepository
	uvStatsRepo repository.DailyCardUVStatsRepository
}

func NewCardAccessService(db *gorm.DB) CardAccessService {
	return &cardAccessService{
		accessRepo:  repository.NewCardAccessRepository(db),
		uvStatsRepo: repository.NewDailyCardUVStatsRepository(db),
	}
}

func (s *cardAccessService) RecordAccess(cardID uint, cardType string, ip, ua, referer string) error {
	access := &model.CardAccess{
		CardID:     cardID,
		CardType:   cardType,
		IPAddress:  ip,
		UserAgent:  ua,
		Referer:    referer,
		DeviceType: utils.ParseDeviceType(ua),
		Browser:    utils.ParseBrowser(ua),
		OS:         utils.ParseOS(ua),
	}

	if err := s.accessRepo.Create(access); err != nil {
		return err
	}

	today := time.Now().Format("2006-01-02")

	stats, err := s.uvStatsRepo.GetByCardIDAndDate(cardID, cardType, today)
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}

	if stats == nil {
		stats = &model.DailyCardUVStats{
			CardID:   cardID,
			CardType: cardType,
			Date:     today,
			UVCount:  1,
			PVCount:  1,
		}
		return s.uvStatsRepo.Create(stats)
	}

	stats.PVCount++

	hasAccessToday, err := s.accessRepo.HasAccessToday(cardID, ip)
	if err != nil {
		return err
	}
	if !hasAccessToday {
		stats.UVCount++
	}

	return s.uvStatsRepo.Update(stats)
}

func (s *cardAccessService) GetCardUVStats(cardID uint, cardType string, startDate, endDate time.Time) (*CardStatsResponse, error) {
	totalUV, err := s.accessRepo.CountDistinctIP(cardID, cardType, startDate, endDate)
	if err != nil {
		return nil, err
	}

	totalPV, err := s.accessRepo.CountAccess(cardID, cardType, startDate, endDate)
	if err != nil {
		return nil, err
	}

	today := time.Now().Format("2006-01-02")
	todayStats, _ := s.uvStatsRepo.GetByCardIDAndDate(cardID, cardType, today)

	return &CardStatsResponse{
		CardID:    cardID,
		CardType:  cardType,
		TotalUV:   totalUV,
		TotalPV:   totalPV,
		TodayUV:   todayStats.UVCount,
		TodayPV:   todayStats.PVCount,
		StartDate: startDate,
		EndDate:   endDate,
	}, nil
}

func (s *cardAccessService) GetDailyUVStats(cardID uint, cardType string) ([]DailyStats, error) {
	stats, err := s.uvStatsRepo.GetByCardID(cardID, cardType)
	if err != nil {
		return nil, err
	}

	var result []DailyStats
	for _, stat := range stats {
		result = append(result, DailyStats{
			Date:    stat.Date,
			UVCount: stat.UVCount,
			PVCount: stat.PVCount,
		})
	}

	return result, nil
}

func (s *cardAccessService) GetTodayUV(cardID uint, cardType string) (int, error) {
	today := time.Now().Format("2006-01-02")
	stats, err := s.uvStatsRepo.GetByCardIDAndDate(cardID, cardType, today)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, nil
		}
		return 0, err
	}

	return stats.UVCount, nil
}
