package repository

// asset_bundle_ab_test_repo.go 资产包 A/B 实验仓储
//
// 五层架构归属: L3 Repository 层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1) §2.3.3 §3.5
//
// 职责：
//   - AssetBundleABTest CRUD（含按状态/scenario 查询、Bandit 累计更新）
//   - 不含业务逻辑，仅数据访问
//
// 关键接口：
//   - IncrementSamples      原子累加样本数与奖励值（避免 lost-update）
//   - FindRunningByBaseline 按 baseline 查询在跑实验（防止重复实验）

import (
	"context"
	"fmt"
	"time"

	"marketing/internal/model"

	"gorm.io/gorm"
)

// AssetBundleABTestRepository 资产包 A/B 实验仓储接口
type AssetBundleABTestRepository interface {
	// Create 创建实验（实验 ID 全局唯一）
	Create(ctx context.Context, m *model.AssetBundleABTest) error
	// GetByExperimentID 按实验 ID 查询
	GetByExperimentID(ctx context.Context, experimentID string) (*model.AssetBundleABTest, error)
	// FindRunningByBaseline 按 baseline 查询在跑实验（同一 baseline 同时只能跑一个 candidate）
	FindRunningByBaseline(ctx context.Context, baselineAssetID string) (*model.AssetBundleABTest, error)
	// ListByStatus 按状态查询
	ListByStatus(ctx context.Context, status model.AssetBundleABTestStatus, limit int) ([]*model.AssetBundleABTest, error)
	// ListRunningByScenario 按场景查询在跑实验（用于 Bandit 收敛检查）
	ListRunningByScenario(ctx context.Context, scenario string, limit int) ([]*model.AssetBundleABTest, error)
	// IncrementSamples 原子累加样本数与奖励值
	// arm: "baseline" 或 "candidate"
	// deltaSamples: 样本增量；deltaReward: 奖励增量
	IncrementSamples(ctx context.Context, experimentID string, arm string, deltaSamples int, deltaReward float64) error
	// UpdateStatus 更新状态（含 winner_arm / converged_at / completed_at 等字段）
	UpdateStatus(ctx context.Context, experimentID string, status model.AssetBundleABTestStatus, extraUpdates map[string]any) error
	// CountByStatus 按状态分组统计
	CountByStatus(ctx context.Context) (map[model.AssetBundleABTestStatus]int64, error)
	// ListConvergedPendingAction 查询已收敛但未完成（promote/rollback）的实验
	ListConvergedPendingAction(ctx context.Context, limit int) ([]*model.AssetBundleABTest, error)
}

type assetBundleABTestRepo struct {
	db *gorm.DB
}

// NewAssetBundleABTestRepository 创建资产包 A/B 实验仓储
func NewAssetBundleABTestRepository(db *gorm.DB) AssetBundleABTestRepository {
	return &assetBundleABTestRepo{db: db}
}

// Create 创建实验
func (r *assetBundleABTestRepo) Create(ctx context.Context, m *model.AssetBundleABTest) error {
	if m == nil {
		return fmt.Errorf("ab_test is nil")
	}
	if m.ExperimentID == "" {
		return fmt.Errorf("experiment_id is empty")
	}
	if m.StartedAt.IsZero() {
		m.StartedAt = time.Now()
	}
	return r.db.WithContext(ctx).Create(m).Error
}

// GetByExperimentID 按实验 ID 查询
func (r *assetBundleABTestRepo) GetByExperimentID(ctx context.Context, experimentID string) (*model.AssetBundleABTest, error) {
	var t model.AssetBundleABTest
	err := r.db.WithContext(ctx).Where("experiment_id = ?", experimentID).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// FindRunningByBaseline 按 baseline 查询在跑实验
//
// 用途：同一 baseline 资产包同时只能跑一个 candidate 实验，避免流量撕裂
func (r *assetBundleABTestRepo) FindRunningByBaseline(ctx context.Context, baselineAssetID string) (*model.AssetBundleABTest, error) {
	var t model.AssetBundleABTest
	err := r.db.WithContext(ctx).
		Where("baseline_asset_id = ? AND status = ?", baselineAssetID, model.ABTestStatusRunning).
		Order("started_at DESC").
		First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListByStatus 按状态查询
func (r *assetBundleABTestRepo) ListByStatus(ctx context.Context, status model.AssetBundleABTestStatus, limit int) ([]*model.AssetBundleABTest, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var list []*model.AssetBundleABTest
	err := r.db.WithContext(ctx).
		Where("status = ?", status).
		Order("started_at DESC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

// ListRunningByScenario 按场景查询在跑实验
func (r *assetBundleABTestRepo) ListRunningByScenario(ctx context.Context, scenario string, limit int) ([]*model.AssetBundleABTest, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var list []*model.AssetBundleABTest
	q := r.db.WithContext(ctx).Where("status = ?", model.ABTestStatusRunning)
	if scenario != "" {
		q = q.Where("scenario = ?", scenario)
	}
	err := q.Order("started_at ASC").Limit(limit).Find(&list).Error
	return list, err
}

// IncrementSamples 原子累加样本数与奖励值
//
// 使用 PostgreSQL 行级锁 + 原子 UPDATE，避免 Bandit 并发更新 lost-update。
// 采用 switch + 硬编码 SQL 而非 fmt.Sprintf 拼接列名，
// 从源头消除 SQL 注入风险（即使 arm 校验被意外移除也不会注入）。
//
// 对应 SQL：
//
//	UPDATE asset_bundle_ab_tests
//	SET baseline_samples = baseline_samples + $1,
//	    baseline_reward  = baseline_reward  + $2,
//	    updated_at       = NOW()
//	WHERE experiment_id = $3
func (r *assetBundleABTestRepo) IncrementSamples(ctx context.Context, experimentID string, arm string, deltaSamples int, deltaReward float64) error {
	if arm != "baseline" && arm != "candidate" {
		return fmt.Errorf("invalid arm: %s (must be baseline or candidate)", arm)
	}
	switch arm {
	case "baseline":
		const sql = `UPDATE asset_bundle_ab_tests
			SET baseline_samples = baseline_samples + ?,
			    baseline_reward  = baseline_reward  + ?,
			    updated_at       = NOW()
			WHERE experiment_id = ?`
		return r.db.WithContext(ctx).Exec(sql, deltaSamples, deltaReward, experimentID).Error
	case "candidate":
		const sql = `UPDATE asset_bundle_ab_tests
			SET candidate_samples = candidate_samples + ?,
			    candidate_reward  = candidate_reward  + ?,
			    updated_at        = NOW()
			WHERE experiment_id = ?`
		return r.db.WithContext(ctx).Exec(sql, deltaSamples, deltaReward, experimentID).Error
	default:
		// 防御性兜底：理论不可达（上方校验已拦截）
		return fmt.Errorf("unreachable: invalid arm %q", arm)
	}
}

// UpdateStatus 更新状态（带状态机守卫）
//
// 守卫：WHERE status IN ('running', 'converged')，仅允许从非终态转换。
// 终态（completed/rolled_back）不可再变更，防止状态回归。
//
// 合法转换路径：
//   - running → converged（Bandit 收敛）
//   - converged → completed（promote 或 rollback 完成）
//   - converged → rolled_back（回滚）
func (r *assetBundleABTestRepo) UpdateStatus(ctx context.Context, experimentID string, status model.AssetBundleABTestStatus, extraUpdates map[string]any) error {
	updates := map[string]any{
		"status":     status,
		"updated_at": time.Now(),
	}
	for k, v := range extraUpdates {
		updates[k] = v
	}
	return r.db.WithContext(ctx).Model(&model.AssetBundleABTest{}).
		Where("experiment_id = ? AND status IN ?", experimentID,
			[]model.AssetBundleABTestStatus{model.ABTestStatusRunning, model.ABTestStatusConverged}).
		Updates(updates).Error
}

// CountByStatus 按状态分组统计
func (r *assetBundleABTestRepo) CountByStatus(ctx context.Context) (map[model.AssetBundleABTestStatus]int64, error) {
	type result struct {
		Status model.AssetBundleABTestStatus
		Count  int64
	}
	var results []result
	err := r.db.WithContext(ctx).Model(&model.AssetBundleABTest{}).
		Select("status, COUNT(*) as count").
		Group("status").
		Scan(&results).Error
	if err != nil {
		return nil, err
	}
	out := make(map[model.AssetBundleABTestStatus]int64, len(results))
	for _, r := range results {
		out[r.Status] = r.Count
	}
	return out, nil
}

// ListConvergedPendingAction 查询已收敛但未完成（promote/rollback）的实验
//
// 用途：定时任务扫描，将 converged → completed（执行 promote 或 rollback）
func (r *assetBundleABTestRepo) ListConvergedPendingAction(ctx context.Context, limit int) ([]*model.AssetBundleABTest, error) {
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	var list []*model.AssetBundleABTest
	err := r.db.WithContext(ctx).
		Where("status = ?", model.ABTestStatusConverged).
		Order("converged_at ASC").
		Limit(limit).
		Find(&list).Error
	return list, err
}
