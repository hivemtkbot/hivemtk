package repository

// asset_bundle_candidate_repo.go 资产包候选仓储
//
// 五层架构归属: L3 Repository 层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1) §2.3.2 §3.5
//
// 职责：
//   - AssetBundleCandidate CRUD（含按状态/scenario 查询、批量状态更新）
//   - 不含业务逻辑，仅数据访问

import (
	"context"
	"fmt"
	"time"

	"marketing/internal/model"

	"gorm.io/gorm"
)

// AssetBundleCandidateRepository 资产包候选仓储接口
type AssetBundleCandidateRepository interface {
	// Create 创建候选
	Create(ctx context.Context, m *model.AssetBundleCandidate) error
	// GetByCandidateID 按 candidate_id 查询
	GetByCandidateID(ctx context.Context, candidateID string) (*model.AssetBundleCandidate, error)
	// ListPendingByScenario 查询指定 scenario 下未进入 A/B 的候选（status=candidate）
	ListPendingByScenario(ctx context.Context, scenario string, since time.Time, limit int) ([]*model.AssetBundleCandidate, error)
	// ListByStatus 按状态查询
	ListByStatus(ctx context.Context, status model.AssetBundleCandidateStatus, limit int) ([]*model.AssetBundleCandidate, error)
	// UpdateStatus 更新状态
	UpdateStatus(ctx context.Context, candidateID string, status model.AssetBundleCandidateStatus, extraUpdates map[string]any) error
	// CountToday 今日候选生成统计
	CountToday(ctx context.Context) (map[model.AssetBundleCandidateStatus]int64, error)
	// CountByStatus 总数按状态分组
	CountByStatus(ctx context.Context) (map[model.AssetBundleCandidateStatus]int64, error)
}

type assetBundleCandidateRepo struct {
	db *gorm.DB
}

// NewAssetBundleCandidateRepository 创建资产包候选仓储
func NewAssetBundleCandidateRepository(db *gorm.DB) AssetBundleCandidateRepository {
	return &assetBundleCandidateRepo{db: db}
}

// Create 创建候选
func (r *assetBundleCandidateRepo) Create(ctx context.Context, m *model.AssetBundleCandidate) error {
	if m == nil {
		return fmt.Errorf("candidate is nil")
	}
	if m.CandidateID == "" {
		return fmt.Errorf("candidate_id is empty")
	}
	return r.db.WithContext(ctx).Create(m).Error
}

// GetByCandidateID 按 candidate_id 查询
func (r *assetBundleCandidateRepo) GetByCandidateID(ctx context.Context, candidateID string) (*model.AssetBundleCandidate, error) {
	var c model.AssetBundleCandidate
	err := r.db.WithContext(ctx).Where("candidate_id = ?", candidateID).First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListPendingByScenario 查询指定 scenario 下未进入 A/B 的候选
func (r *assetBundleCandidateRepo) ListPendingByScenario(ctx context.Context, scenario string, since time.Time, limit int) ([]*model.AssetBundleCandidate, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var list []*model.AssetBundleCandidate
	q := r.db.WithContext(ctx).
		Where("status = ?", model.CandidateStatusCandidate).
		Where("created_at >= ?", since)
	if scenario != "" {
		q = q.Where("scenario = ?", scenario)
	}
	err := q.Order("created_at DESC").Limit(limit).Find(&list).Error
	return list, err
}

// ListByStatus 按状态查询
func (r *assetBundleCandidateRepo) ListByStatus(ctx context.Context, status model.AssetBundleCandidateStatus, limit int) ([]*model.AssetBundleCandidate, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var list []*model.AssetBundleCandidate
	err := r.db.WithContext(ctx).
		Where("status = ?", status).
		Order("created_at DESC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

// UpdateStatus 更新状态（带状态机守卫）
//
// 守卫：WHERE status IN ('candidate', 'ab_testing')，仅允许从非终态转换。
// 终态（promoted/rejected）不可再变更，防止状态回归。
//
// 合法转换路径：
//   - candidate → ab_testing（进入 A/B 实验）
//   - ab_testing → promoted（A/B 胜出，升级为 active）
//   - ab_testing → rejected（A/B 失败或人工拒绝）
func (r *assetBundleCandidateRepo) UpdateStatus(ctx context.Context, candidateID string, status model.AssetBundleCandidateStatus, extraUpdates map[string]any) error {
	updates := map[string]any{
		"status":     status,
		"updated_at": time.Now(),
	}
	for k, v := range extraUpdates {
		updates[k] = v
	}
	return r.db.WithContext(ctx).Model(&model.AssetBundleCandidate{}).
		Where("candidate_id = ? AND status IN ?", candidateID,
			[]model.AssetBundleCandidateStatus{model.CandidateStatusCandidate, model.CandidateStatusABTesting}).
		Updates(updates).Error
}

// CountToday 今日候选生成统计
//
// 时区说明：参见 self_learning_repo.go 的 startOfTodayShanghai 注释。
// 不能用 time.Now().Truncate(24h) —— 那是按 UTC 截断，
// 在 UTC+8 时区下会把"今日 0 点"误算成 UTC 16:00（前一天）。
func (r *assetBundleCandidateRepo) CountToday(ctx context.Context) (map[model.AssetBundleCandidateStatus]int64, error) {
	type result struct {
		Status model.AssetBundleCandidateStatus
		Count  int64
	}
	var results []result
	start := startOfTodayShanghai()
	err := r.db.WithContext(ctx).Model(&model.AssetBundleCandidate{}).
		Select("status, COUNT(*) as count").
		Where("created_at >= ?", start).
		Group("status").
		Scan(&results).Error
	if err != nil {
		return nil, err
	}
	out := make(map[model.AssetBundleCandidateStatus]int64, len(results))
	for _, r := range results {
		out[r.Status] = r.Count
	}
	return out, nil
}

// CountByStatus 总数按状态分组
func (r *assetBundleCandidateRepo) CountByStatus(ctx context.Context) (map[model.AssetBundleCandidateStatus]int64, error) {
	type result struct {
		Status model.AssetBundleCandidateStatus
		Count  int64
	}
	var results []result
	err := r.db.WithContext(ctx).Model(&model.AssetBundleCandidate{}).
		Select("status, COUNT(*) as count").
		Group("status").
		Scan(&results).Error
	if err != nil {
		return nil, err
	}
	out := make(map[model.AssetBundleCandidateStatus]int64, len(results))
	for _, r := range results {
		out[r.Status] = r.Count
	}
	return out, nil
}
