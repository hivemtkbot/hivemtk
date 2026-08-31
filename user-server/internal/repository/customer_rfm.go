package repository

import (
	"context"
	"errors"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
	"time"

	"gorm.io/gorm"
)

// CustomerRFMRepository 客户 RFM 仓库
type CustomerRFMRepository interface {
	Upsert(ctx context.Context, rfm *model.CustomerRFM) error
	GetByCustomerID(ctx context.Context, customerID string) (*model.CustomerRFM, error)
	ListBySegment(ctx context.Context, segment string, page, pageSize int) ([]*model.CustomerRFM, int64, error)
	ListChurnCandidates(ctx context.Context, threshold int, limit int) ([]*model.CustomerRFM, error)
	CountBySegment(ctx context.Context) (map[string]int64, error)
	DeleteByCustomerID(ctx context.Context, customerID string) error
}

type customerRFMRepo struct {
	db *gorm.DB
}

func NewCustomerRFMRepository() CustomerRFMRepository {
	return &customerRFMRepo{db: _db.GetDB()}
}

func NewCustomerRFMRepositoryWithDB(db *gorm.DB) CustomerRFMRepository {
	return &customerRFMRepo{db: db}
}

func (r *customerRFMRepo) Upsert(ctx context.Context, rfm *model.CustomerRFM) error {
	if rfm.CustomerID == "" {
		return errors.New("customer_id 不能为空")
	}
	var existing model.CustomerRFM
	err := r.db.Where("customer_id = ?", rfm.CustomerID).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(rfm).Error
	}
	rfm.ID = existing.ID
	rfm.CreatedAt = existing.CreatedAt
	rfm.ComputedAt = time.Now()
	return r.db.Save(rfm).Error
}

func (r *customerRFMRepo) GetByCustomerID(ctx context.Context, customerID string) (*model.CustomerRFM, error) {
	var rfm model.CustomerRFM
	if err := r.db.Where("customer_id = ?", customerID).First(&rfm).Error; err != nil {
		return nil, err
	}
	return &rfm, nil
}

func (r *customerRFMRepo) ListBySegment(ctx context.Context, segment string, page, pageSize int) ([]*model.CustomerRFM, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	var rfms []*model.CustomerRFM
	var total int64
	q := r.db.Model(&model.CustomerRFM{})
	if segment != "" {
		q = q.Where("segment = ?", segment)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("composite_score DESC, monetary_total DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&rfms).Error; err != nil {
		return nil, 0, err
	}
	return rfms, total, nil
}

func (r *customerRFMRepo) ListChurnCandidates(ctx context.Context, threshold int, limit int) ([]*model.CustomerRFM, error) {
	if limit < 1 || limit > 500 {
		limit = 50
	}
	var rfms []*model.CustomerRFM
	err := r.db.Where("churn_risk_level = ? AND churn_score >= ?", "high", threshold).
		Order("churn_score DESC, recency_days DESC").
		Limit(limit).Find(&rfms).Error
	return rfms, err
}

func (r *customerRFMRepo) CountBySegment(ctx context.Context) (map[string]int64, error) {
	type row struct {
		Segment string
		Cnt     int64
	}
	var rows []row
	if err := r.db.Model(&model.CustomerRFM{}).
		Select("segment, COUNT(*) as cnt").Group("segment").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.Segment] = r.Cnt
	}
	return out, nil
}

func (r *customerRFMRepo) DeleteByCustomerID(ctx context.Context, customerID string) error {
	return r.db.Where("customer_id = ?", customerID).Delete(&model.CustomerRFM{}).Error
}

// RecoveryQueueRepository 流失挽回队列仓库
type RecoveryQueueRepository interface {
	Create(ctx context.Context, item *model.RecoveryQueue) error
	Update(ctx context.Context, item *model.RecoveryQueue) error
	GetByID(ctx context.Context, id uint64) (*model.RecoveryQueue, error)
	GetActiveByCustomerID(ctx context.Context, customerID string) (*model.RecoveryQueue, error)
	ListByStage(ctx context.Context, stage string, page, pageSize int) ([]*model.RecoveryQueue, int64, error)
	ListReadyForAttempt(ctx context.Context, now time.Time, limit int) ([]*model.RecoveryQueue, error)
	MarkAttempt(ctx context.Context, id uint64, channel, result string, nextAt *time.Time) error
	MarkStage(ctx context.Context, id uint64, stage string) error
	CountByStage(ctx context.Context) (map[string]int64, error)
	Delete(ctx context.Context, id uint64) error
}

type recoveryQueueRepo struct {
	db *gorm.DB
}

func NewRecoveryQueueRepository() RecoveryQueueRepository {
	return &recoveryQueueRepo{db: _db.GetDB()}
}

func NewRecoveryQueueRepositoryWithDB(db *gorm.DB) RecoveryQueueRepository {
	return &recoveryQueueRepo{db: db}
}

func (r *recoveryQueueRepo) Create(ctx context.Context, item *model.RecoveryQueue) error {
	if item.CustomerID == "" {
		return errors.New("customer_id 不能为空")
	}
	existing, err := r.GetActiveByCustomerID(ctx, item.CustomerID)
	if err == nil && existing != nil {
		return errors.New("客户已在挽回队列中")
	}
	return r.db.Create(item).Error
}

func (r *recoveryQueueRepo) Update(ctx context.Context, item *model.RecoveryQueue) error {
	if item.ID == 0 {
		return errors.New("id 不能为空")
	}
	return r.db.Save(item).Error
}

func (r *recoveryQueueRepo) GetByID(ctx context.Context, id uint64) (*model.RecoveryQueue, error) {
	var item model.RecoveryQueue
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *recoveryQueueRepo) GetActiveByCustomerID(ctx context.Context, customerID string) (*model.RecoveryQueue, error) {
	var item model.RecoveryQueue
	if err := r.db.Where("customer_id = ? AND stage IN ?", customerID,
		[]string{model.RecoveryStageQueued, model.RecoveryStageRunning}).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *recoveryQueueRepo) ListByStage(ctx context.Context, stage string, page, pageSize int) ([]*model.RecoveryQueue, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	var items []*model.RecoveryQueue
	var total int64
	q := r.db.Model(&model.RecoveryQueue{})
	if stage != "" {
		q = q.Where("stage = ?", stage)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("priority ASC, created_at ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *recoveryQueueRepo) ListReadyForAttempt(ctx context.Context, now time.Time, limit int) ([]*model.RecoveryQueue, error) {
	if limit < 1 || limit > 500 {
		limit = 50
	}
	var items []*model.RecoveryQueue
	err := r.db.Where("stage = ? AND attempts < max_attempts AND (next_attempt_at IS NULL OR next_attempt_at <= ?)",
		model.RecoveryStageQueued, now).
		Order("priority ASC, next_attempt_at ASC NULLS FIRST").
		Limit(limit).Find(&items).Error
	return items, err
}

func (r *recoveryQueueRepo) MarkAttempt(ctx context.Context, id uint64, channel, result string, nextAt *time.Time) error {
	updates := map[string]any{
		"attempts":        gorm.Expr("attempts + 1"),
		"last_attempt_at": time.Now(),
		"last_channel":    channel,
		"last_result":     result,
	}
	if nextAt != nil {
		updates["next_attempt_at"] = *nextAt
	}
	return r.db.Model(&model.RecoveryQueue{}).Where("id = ?", id).Updates(updates).Error
}

func (r *recoveryQueueRepo) MarkStage(ctx context.Context, id uint64, stage string) error {
	return r.db.Model(&model.RecoveryQueue{}).Where("id = ?", id).Update("stage", stage).Error
}

func (r *recoveryQueueRepo) CountByStage(ctx context.Context) (map[string]int64, error) {
	type row struct {
		Stage string
		Cnt   int64
	}
	var rows []row
	if err := r.db.Model(&model.RecoveryQueue{}).
		Select("stage, COUNT(*) as cnt").Group("stage").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.Stage] = r.Cnt
	}
	return out, nil
}

func (r *recoveryQueueRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.Delete(&model.RecoveryQueue{}, id).Error
}
