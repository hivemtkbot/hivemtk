package repository

import (
	"context"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// PromptRepo PromptCandidate + PromptABTest 仓库
type PromptRepo struct {
	db *gorm.DB
}

// NewPromptRepo 构造
func NewPromptRepo() *PromptRepo {
	return &PromptRepo{db: _db.GetDB()}
}

// NewPromptRepoWithDB 注入 DB（测试用）
func NewPromptRepoWithDB(db *gorm.DB) *PromptRepo {
	return &PromptRepo{db: db}
}

// ListVersions 按 id / sop_id / sop_node_id 查询 prompt 版本列表
// idStr: 既作为字符串匹配 id，也作为 uint 匹配 sop_id
func (r *PromptRepo) ListVersions(ctx context.Context, idStr string, sopID uint, sopNodeID string, status string) ([]model.PromptCandidate, error) {
	q := r.db.WithContext(ctx).Model(&model.PromptCandidate{}).
		Where("id = ? OR sop_id = ? OR sop_node_id = ?", idStr, sopID, idStr)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var versions []model.PromptCandidate
	if err := q.Order("created_at DESC").Find(&versions).Error; err != nil {
		return nil, err
	}
	return versions, nil
}

// RetireActiveBySOPNode 将同一 sop_node_id 下状态为 active 的版本降为 retired
func (r *PromptRepo) RetireActiveBySOPNode(ctx context.Context, sopNodeID string) error {
	return r.db.WithContext(ctx).
		Model(&model.PromptCandidate{}).
		Where("sop_node_id = ? AND status = ?", sopNodeID, model.PromptCandidateStatusActive).
		Update("status", model.PromptCandidateStatusRetired).Error
}

// CreateCandidate 创建新版本
func (r *PromptRepo) CreateCandidate(ctx context.Context, c *model.PromptCandidate) error {
	return r.db.WithContext(ctx).Create(c).Error
}

// ListABTests 列出所有 A/B 实验（可按 status 过滤）
func (r *PromptRepo) ListABTests(ctx context.Context, status string) ([]model.PromptABTest, error) {
	q := r.db.WithContext(ctx).Model(&model.PromptABTest{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var experiments []model.PromptABTest
	if err := q.Order("created_at DESC").Find(&experiments).Error; err != nil {
		return nil, err
	}
	return experiments, nil
}
