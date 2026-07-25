package repository

// knowledge_chunk_quality_repo.go 知识库语料质量字段仓储
//
// 五层架构归属: L3 Repository 层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1) §2.3.4 §7.3
//
// 职责：
//   - 操作 knowledge_chunks 表的 6 个质量字段（quality_score/quality_label/
//     low_quality_hits/champion_hits/source_session_ids/last_reward_at）
//   - 由于 KnowledgeChunkExt 与 KnowledgeChunk 共享同一张表，使用 Raw SQL 避免冲突
//   - 提供 RAG 自我矫正所需的数据访问：低质标记/销冠补录/归档/降权

import (
	"context"
	"time"

	"marketing/internal/model"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// KnowledgeChunkExtRepository 知识库语料扩展字段仓储接口
type KnowledgeChunkExtRepository interface {
	// GetExt 获取语料的扩展字段（按 chunk id）
	GetExt(ctx context.Context, chunkID uint64) (*model.KnowledgeChunkExt, error)
	// IncrementLowQualityHits 累加低质命中次数（并自动更新 quality_label）
	// 触发规则：
	//   - hits >= 10 → archived（从召回池剔除）
	//   - hits >= 3  → low_quality
	IncrementLowQualityHits(ctx context.Context, chunkID uint64, delta int, sourceSessionID string) error
	// IncrementChampionHits 累加销冠命中次数（并自动升级 quality_label=champion）
	IncrementChampionHits(ctx context.Context, chunkID uint64, delta int, reward float64, sourceSessionID string) error
	// MarkArchived 直接归档（用于人工或极端异常场景）
	MarkArchived(ctx context.Context, chunkID uint64, reason string) error
	// ListLowQuality 列出低质语料（low_quality_hits >= 阈值）
	ListLowQuality(ctx context.Context, threshold int, limit int) ([]uint64, error)
	// ListChampion 列出销冠语料（quality_label=champion，按 champion_hits DESC）
	ListChampion(ctx context.Context, limit int) ([]uint64, error)
	// AddSourceSessionID 追加来源会话 ID 到 source_session_ids 数组
	AddSourceSessionID(ctx context.Context, chunkID uint64, sessionID string) error
	// UpdateQualityScore 更新质量分（销冠补录 +reward，低质标记 -reward）
	UpdateQualityScore(ctx context.Context, chunkID uint64, delta float64) error
	// BatchGetExt 批量获取扩展字段
	BatchGetExt(ctx context.Context, chunkIDs []uint64) (map[uint64]*model.KnowledgeChunkExt, error)
}

type knowledgeChunkExtRepo struct {
	db *gorm.DB
}

// NewKnowledgeChunkExtRepository 创建知识库语料扩展仓储
func NewKnowledgeChunkExtRepository(db *gorm.DB) KnowledgeChunkExtRepository {
	return &knowledgeChunkExtRepo{db: db}
}

// GetExt 获取语料的扩展字段
func (r *knowledgeChunkExtRepo) GetExt(ctx context.Context, chunkID uint64) (*model.KnowledgeChunkExt, error) {
	var ext model.KnowledgeChunkExt
	err := r.db.WithContext(ctx).
		Table("knowledge_chunks").
		Select("quality_score, quality_label, low_quality_hits, champion_hits, source_session_ids, last_reward_at").
		Where("id = ?", chunkID).
		First(&ext).Error
	if err != nil {
		return nil, err
	}
	return &ext, nil
}

// BatchGetExt 批量获取扩展字段
func (r *knowledgeChunkExtRepo) BatchGetExt(ctx context.Context, chunkIDs []uint64) (map[uint64]*model.KnowledgeChunkExt, error) {
	if len(chunkIDs) == 0 {
		return map[uint64]*model.KnowledgeChunkExt{}, nil
	}
	type row struct {
		ID              uint64
		QualityScore    float64
		QualityLabel    string
		LowQualityHits  int
		ChampionHits    int
		SourceSessionIDs pq.StringArray
		LastRewardAt    *time.Time
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("knowledge_chunks").
		Select("id, quality_score, quality_label, low_quality_hits, champion_hits, source_session_ids, last_reward_at").
		Where("id IN ?", chunkIDs).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[uint64]*model.KnowledgeChunkExt, len(rows))
	for _, row := range rows {
		out[row.ID] = &model.KnowledgeChunkExt{
			QualityScore:     row.QualityScore,
			QualityLabel:     model.KnowledgeChunkQualityLabel(row.QualityLabel),
			LowQualityHits:   row.LowQualityHits,
			ChampionHits:     row.ChampionHits,
			SourceSessionIDs: row.SourceSessionIDs,
			LastRewardAt:     row.LastRewardAt,
		}
	}
	return out, nil
}

// IncrementLowQualityHits 累加低质命中次数
//
// 单条 SQL 完成：累加 + 状态自动升级 + 数组追加 + 时间戳更新
func (r *knowledgeChunkExtRepo) IncrementLowQualityHits(ctx context.Context, chunkID uint64, delta int, sourceSessionID string) error {
	sql := `
		UPDATE knowledge_chunks
		SET low_quality_hits = low_quality_hits + ?,
		    quality_label = CASE
			    WHEN low_quality_hits + ? >= 10 THEN 'archived'
			    WHEN low_quality_hits + ? >= 3  THEN 'low_quality'
			    ELSE quality_label
		    END,
		    source_session_ids = CASE
			    WHEN ? = '' THEN source_session_ids
			    ELSE array_append(source_session_ids, ?)
		    END,
		    last_reward_at = NOW()
		WHERE id = ?
	`
	return r.db.WithContext(ctx).Exec(sql, delta, delta, delta, sourceSessionID, sourceSessionID, chunkID).Error
}

// IncrementChampionHits 累加销冠命中次数
//
// 触发规则：
//   - reward >= 1.5 → quality_label=champion（无论之前是 normal/low_quality）
//   - 累加 quality_score（销冠补录 +reward）
//   - 累加 champion_hits
//   - 追加 source_session_ids
func (r *knowledgeChunkExtRepo) IncrementChampionHits(ctx context.Context, chunkID uint64, delta int, reward float64, sourceSessionID string) error {
	sql := `
		UPDATE knowledge_chunks
		SET champion_hits = champion_hits + ?,
		    quality_score = quality_score + ?,
		    quality_label = CASE
			    WHEN ? >= 1.5 THEN 'champion'
			    ELSE quality_label
		    END,
		    source_session_ids = CASE
			    WHEN ? = '' THEN source_session_ids
			    ELSE array_append(source_session_ids, ?)
		    END,
		    last_reward_at = NOW()
		WHERE id = ?
	`
	return r.db.WithContext(ctx).Exec(sql, delta, reward, reward, sourceSessionID, sourceSessionID, chunkID).Error
}

// MarkArchived 直接归档
func (r *knowledgeChunkExtRepo) MarkArchived(ctx context.Context, chunkID uint64, reason string) error {
	return r.db.WithContext(ctx).Exec(`
		UPDATE knowledge_chunks
		SET quality_label = 'archived',
		    last_reward_at = NOW()
		WHERE id = ?
	`, chunkID).Error
}

// ListLowQuality 列出低质语料
func (r *knowledgeChunkExtRepo) ListLowQuality(ctx context.Context, threshold int, limit int) ([]uint64, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var ids []uint64
	err := r.db.WithContext(ctx).
		Table("knowledge_chunks").
		Select("id").
		Where("low_quality_hits >= ? AND quality_label != 'archived'", threshold).
		Order("low_quality_hits DESC").
		Limit(limit).
		Pluck("id", &ids).Error
	return ids, err
}

// ListChampion 列出销冠语料
func (r *knowledgeChunkExtRepo) ListChampion(ctx context.Context, limit int) ([]uint64, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var ids []uint64
	err := r.db.WithContext(ctx).
		Table("knowledge_chunks").
		Select("id").
		Where("quality_label = ?", model.QualityLabelChampion).
		Order("champion_hits DESC").
		Limit(limit).
		Pluck("id", &ids).Error
	return ids, err
}

// AddSourceSessionID 追加来源会话 ID
func (r *knowledgeChunkExtRepo) AddSourceSessionID(ctx context.Context, chunkID uint64, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	return r.db.WithContext(ctx).Exec(`
		UPDATE knowledge_chunks
		SET source_session_ids = array_append(source_session_ids, ?)
		WHERE id = ?
	`, sessionID, chunkID).Error
}

// UpdateQualityScore 更新质量分
func (r *knowledgeChunkExtRepo) UpdateQualityScore(ctx context.Context, chunkID uint64, delta float64) error {
	return r.db.WithContext(ctx).Exec(`
		UPDATE knowledge_chunks
		SET quality_score = quality_score + ?,
		    last_reward_at = NOW()
		WHERE id = ?
	`, delta, chunkID).Error
}

// 编译期断言
var _ KnowledgeChunkExtRepository = (*knowledgeChunkExtRepo)(nil)
