package repository

import (
	"errors"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
	"time"

	"gorm.io/gorm"
)

// ClueScoreRepository 线索评分仓库
type ClueScoreRepository interface {
	Upsert(score *model.ClueScore) error
	GetByClueID(clueID string) (*model.ClueScore, error)
	ListByGrade(grade string, page, pageSize int) ([]*model.ClueScore, int64, error)
	ListTopByScore(limit int) ([]*model.ClueScore, error)
	DeleteByClueID(clueID string) error
}

type clueScoreRepo struct {
	db *gorm.DB
}

// NewClueScoreRepository 创建线索评分仓库
func NewClueScoreRepository() ClueScoreRepository {
	return &clueScoreRepo{db: _db.GetDB()}
}

// NewClueScoreRepositoryWithDB 测试用 - 自定义 DB
func NewClueScoreRepositoryWithDB(db *gorm.DB) ClueScoreRepository {
	return &clueScoreRepo{db: db}
}

func (r *clueScoreRepo) Upsert(score *model.ClueScore) error {
	if score.ClueID == "" {
		return errors.New("clue_id 不能为空")
	}
	var existing model.ClueScore
	err := r.db.Where("clue_id = ?", score.ClueID).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(score).Error
	}
	score.ID = existing.ID
	score.CreatedAt = existing.CreatedAt
	score.ScoredAt = time.Now()
	return r.db.Save(score).Error
}

func (r *clueScoreRepo) GetByClueID(clueID string) (*model.ClueScore, error) {
	var score model.ClueScore
	if err := r.db.Where("clue_id = ?", clueID).First(&score).Error; err != nil {
		return nil, err
	}
	return &score, nil
}

func (r *clueScoreRepo) ListByGrade(grade string, page, pageSize int) ([]*model.ClueScore, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	var scores []*model.ClueScore
	var total int64
	q := r.db.Model(&model.ClueScore{})
	if grade != "" {
		q = q.Where("grade = ?", grade)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("total_score DESC, scored_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&scores).Error; err != nil {
		return nil, 0, err
	}
	return scores, total, nil
}

func (r *clueScoreRepo) ListTopByScore(limit int) ([]*model.ClueScore, error) {
	if limit < 1 || limit > 200 {
		limit = 20
	}
	var scores []*model.ClueScore
	if err := r.db.Order("total_score DESC, scored_at DESC").Limit(limit).Find(&scores).Error; err != nil {
		return nil, err
	}
	return scores, nil
}

func (r *clueScoreRepo) DeleteByClueID(clueID string) error {
	return r.db.Where("clue_id = ?", clueID).Delete(&model.ClueScore{}).Error
}

// ClueEngagementRepository 线索互动事件仓库
type ClueEngagementRepository interface {
	Create(evt *model.ClueEngagementEvent) error
	CountByClueID(clueID string, since time.Time) (int64, error)
	CountByType(clueID string) (map[string]int64, error)
	LastByClueID(clueID string, limit int) ([]*model.ClueEngagementEvent, error)
}

type clueEngagementRepo struct {
	db *gorm.DB
}

func NewClueEngagementRepository() ClueEngagementRepository {
	return &clueEngagementRepo{db: _db.GetDB()}
}

func NewClueEngagementRepositoryWithDB(db *gorm.DB) ClueEngagementRepository {
	return &clueEngagementRepo{db: db}
}

func (r *clueEngagementRepo) Create(evt *model.ClueEngagementEvent) error {
	if evt.ClueID == "" {
		return errors.New("clue_id 不能为空")
	}
	return r.db.Create(evt).Error
}

func (r *clueEngagementRepo) CountByClueID(clueID string, since time.Time) (int64, error) {
	var count int64
	err := r.db.Model(&model.ClueEngagementEvent{}).
		Where("clue_id = ? AND created_at >= ?", clueID, since).Count(&count).Error
	return count, err
}

func (r *clueEngagementRepo) CountByType(clueID string) (map[string]int64, error) {
	type resultRow struct {
		EventType string
		Cnt       int64
	}
	var rows []resultRow
	if err := r.db.Model(&model.ClueEngagementEvent{}).
		Select("event_type, COUNT(*) as cnt").
		Where("clue_id = ?", clueID).
		Group("event_type").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.EventType] = r.Cnt
	}
	return out, nil
}

func (r *clueEngagementRepo) LastByClueID(clueID string, limit int) ([]*model.ClueEngagementEvent, error) {
	if limit < 1 || limit > 100 {
		limit = 10
	}
	var evts []*model.ClueEngagementEvent
	err := r.db.Where("clue_id = ?", clueID).Order("created_at DESC").Limit(limit).Find(&evts).Error
	return evts, err
}
