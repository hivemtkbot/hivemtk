package repository

import (
	"context"
	"errors"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
	"time"

	"gorm.io/gorm"
)

// ClueScoreRepository 线索评分仓库
type ClueScoreRepository interface {
	Upsert(ctx context.Context, score *model.ClueScore) error
	GetByClueID(ctx context.Context, clueID string) (*model.ClueScore, error)
	ListByGrade(ctx context.Context, grade string, page, pageSize int) ([]*model.ClueScore, int64, error)
	ListTopByScore(ctx context.Context, limit int) ([]*model.ClueScore, error)
	DeleteByClueID(ctx context.Context, clueID string) error
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

func (r *clueScoreRepo) Upsert(ctx context.Context, score *model.ClueScore) error {
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

func (r *clueScoreRepo) GetByClueID(ctx context.Context, clueID string) (*model.ClueScore, error) {
	var score model.ClueScore
	if err := r.db.Where("clue_id = ?", clueID).First(&score).Error; err != nil {
		return nil, err
	}
	return &score, nil
}

func (r *clueScoreRepo) ListByGrade(ctx context.Context, grade string, page, pageSize int) ([]*model.ClueScore, int64, error) {
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

func (r *clueScoreRepo) ListTopByScore(ctx context.Context, limit int) ([]*model.ClueScore, error) {
	if limit < 1 || limit > 200 {
		limit = 20
	}
	var scores []*model.ClueScore
	if err := r.db.Order("total_score DESC, scored_at DESC").Limit(limit).Find(&scores).Error; err != nil {
		return nil, err
	}
	return scores, nil
}

func (r *clueScoreRepo) DeleteByClueID(ctx context.Context, clueID string) error {
	return r.db.Where("clue_id = ?", clueID).Delete(&model.ClueScore{}).Error
}

// ClueEngagementRepository 线索互动事件仓库
type ClueEngagementRepository interface {
	Create(ctx context.Context, evt *model.ClueEngagementEvent) error
	CountByClueID(ctx context.Context, clueID string, since time.Time) (int64, error)
	CountByType(ctx context.Context, clueID string) (map[string]int64, error)
	LastByClueID(ctx context.Context, clueID string, limit int) ([]*model.ClueEngagementEvent, error)
	// CountByClueIDsBatch 批量统计多个 clue_id 在 since 之后的互动事件数（CC-P2 N+1 优化）
	// 替代「for clue → CountByClueID」N+1；返回 map[clueID]count，未命中为 0。
	CountByClueIDsBatch(ctx context.Context, clueIDs []string, since time.Time) (map[string]int64, error)
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

func (r *clueEngagementRepo) Create(ctx context.Context, evt *model.ClueEngagementEvent) error {
	if evt.ClueID == "" {
		return errors.New("clue_id 不能为空")
	}
	return r.db.Create(evt).Error
}

func (r *clueEngagementRepo) CountByClueID(ctx context.Context, clueID string, since time.Time) (int64, error) {
	var count int64
	err := r.db.Model(&model.ClueEngagementEvent{}).
		Where("clue_id = ? AND created_at >= ?", clueID, since).Count(&count).Error
	return count, err
}

// CountByClueIDsBatch 批量按 clue_id 统计 since 之后的互动事件数（CC-P2 N+1 优化）
//
// 单次 SQL：SELECT clue_id, COUNT(*) FROM clue_engagement_events
// WHERE clue_id IN (...) AND created_at >= ? GROUP BY clue_id
//
// 返回 map[clueID]count，未出现在 GROUP BY 结果中的 clueID 默认 0。
// 入参 clueIDs 去重 + 跳过空串。
func (r *clueEngagementRepo) CountByClueIDsBatch(ctx context.Context, clueIDs []string, since time.Time) (map[string]int64, error) {
	result := make(map[string]int64, len(clueIDs))
	if len(clueIDs) == 0 {
		return result, nil
	}
	unique := make([]string, 0, len(clueIDs))
	seen := make(map[string]struct{}, len(clueIDs))
	for _, id := range clueIDs {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return result, nil
	}
	type row struct {
		ClueID string
		Cnt    int64
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Model(&model.ClueEngagementEvent{}).
		Select("clue_id, COUNT(*) as cnt").
		Where("clue_id IN ? AND created_at >= ?", unique, since).
		Group("clue_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		result[r.ClueID] = r.Cnt
	}
	return result, nil
}

func (r *clueEngagementRepo) CountByType(ctx context.Context, clueID string) (map[string]int64, error) {
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

func (r *clueEngagementRepo) LastByClueID(ctx context.Context, clueID string, limit int) ([]*model.ClueEngagementEvent, error) {
	if limit < 1 || limit > 100 {
		limit = 10
	}
	var evts []*model.ClueEngagementEvent
	err := r.db.Where("clue_id = ?", clueID).Order("created_at DESC").Limit(limit).Find(&evts).Error
	return evts, err
}
