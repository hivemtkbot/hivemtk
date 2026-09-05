package repository

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"hivemtk-user/internal/model"
)

// ChurnScoreRepository D22b：churn_scores 数据访问（纯 GORM，无业务判断）
type ChurnScoreRepository struct {
	db *gorm.DB
}

func NewChurnScoreRepository(db *gorm.DB) *ChurnScoreRepository {
	return &ChurnScoreRepository{db: db}
}

// UpsertBatch 周批全量重算 upsert（customer_key 冲突时整行覆盖，computed_at 刷新）
func (r *ChurnScoreRepository) UpsertBatch(ctx context.Context, rows []model.ChurnScore) error {
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "customer_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"x", "tx", "t_obs", "p_alive", "expected_purchases_30d", "params", "stats_count", "computed_at", "updated_at"}),
	}).CreateInBatches(&rows, 500).Error
}

// ListByPAliveBelow 返回 P(alive) 低于阈值的高流失风险客户（触达管线消费）
func (r *ChurnScoreRepository) ListByPAliveBelow(ctx context.Context, threshold float64, limit int) ([]model.ChurnScore, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows []model.ChurnScore
	err := r.db.WithContext(ctx).
		Where("p_alive < ?", threshold).
		Order("p_alive ASC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

// Count 当前评分行数（job 空跑判定/可观测）
func (r *ChurnScoreRepository) Count(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.ChurnScore{}).Count(&n).Error
	return n, err
}
