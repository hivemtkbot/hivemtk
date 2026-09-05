package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// KBDocumentChunk 知识库文档切片模型
// 实际生产环境可通过 kb_workspace / ai_agent_rag_repository 统一管理；
// 此处为 G12 增量更新独立维护的最小切片结构（存 document_id 级别的增量变更）。
type KBDocumentChunk struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	DocumentID    uint      `gorm:"index;not null" json:"document_id"`
	ChunkIndex    int       `gorm:"not null;default:0" json:"chunk_index"`
	ContentHash   string    `gorm:"type:varchar(64);index;not null" json:"content_hash"`
	ChunkContent  string    `gorm:"type:text" json:"chunk_content"`
	EmbeddingHash string    `gorm:"type:varchar(64);default:''" json:"embedding_hash"`
	Status        string    `gorm:"type:varchar(20);default:'active';index" json:"status"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (KBDocumentChunk) TableName() string { return "kb_document_chunks" }

// KnowledgeUpdateService 知识库增量更新服务
//
// G12: 竞品标配功能 - 文档更新时只重切片修改过的 chunk，
// 不 rebuild 整个 KB。
//
// 核心逻辑：
//  1. 将文档切分为 chunks（按 500 字窗口）
//  2. 计算每个 chunk 的内容 hash（SHA256）
//  3. 对比旧 chunks：hash 不同的 chunk 删除并重新索引；hash 相同的 chunk 保留
type KnowledgeUpdateService struct {
	db *gorm.DB
}

// NewKnowledgeUpdateService 创建增量更新服务
func NewKnowledgeUpdateService() *KnowledgeUpdateService {
	return &KnowledgeUpdateService{db: db.GetDB()}
}

// NewKnowledgeUpdateServiceWithDB 注入 DB（测试用）
func (s *KnowledgeUpdateService) WithDB(d *gorm.DB) *KnowledgeUpdateService {
	s.db = d
	return s
}

// UpdateDeltaResult 增量更新结果
type UpdateDeltaResult struct {
	DocumentID  uint      `json:"document_id"`
	TotalChunks int       `json:"total_chunks"`
	Unchanged   int       `json:"unchanged"`
	Added       int       `json:"added"`
	Removed     int       `json:"removed"`
	Updated     int       `json:"updated"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
}

// UpdateDocumentDelta 对指定文档执行增量切片更新
// 如果 chunks 表不存在或旧 chunks 为空，退化为全量重建
func (s *KnowledgeUpdateService) UpdateDocumentDelta(ctx context.Context, documentID uint, newContent string) (*UpdateDeltaResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("db 未初始化")
	}
	startedAt := time.Now()
	result := &UpdateDeltaResult{DocumentID: documentID, StartedAt: startedAt}

	newChunks := splitIntoChunks(newContent, 500)
	result.TotalChunks = len(newChunks)
	newHashes := make(map[string]int, len(newChunks))
	for i, c := range newChunks {
		h := contentHash(c)
		newHashes[h] = i
	}

	var oldChunks []KBDocumentChunk
	if err := s.db.WithContext(ctx).
		Where("document_id = ? AND status = ?", documentID, "active").
		Order("chunk_index ASC").
		Find(&oldChunks).Error; err != nil {
		return nil, fmt.Errorf("查询旧 chunks: %w", err)
	}

	if len(oldChunks) == 0 {
		for i, c := range newChunks {
			chunk := KBDocumentChunk{
				DocumentID:   documentID,
				ChunkIndex:   i,
				ContentHash:  contentHash(c),
				ChunkContent: c,
				Status:       "active",
			}
			if err := s.db.WithContext(ctx).Create(&chunk).Error; err != nil {
				logger.Warnf("[KBUpdate] 新建 chunk 失败 doc=%d idx=%d: %v", documentID, i, err)
			}
			result.Added++
		}
		result.FinishedAt = time.Now()
		return result, nil
	}

	oldHashSet := make(map[string]*KBDocumentChunk, len(oldChunks))
	for i := range oldChunks {
		oldHashSet[oldChunks[i].ContentHash] = &oldChunks[i]
	}

	toDelete := make([]uint64, 0)
	for _, oc := range oldChunks {
		if _, ok := newHashes[oc.ContentHash]; !ok {
			toDelete = append(toDelete, oc.ID)
		}
	}
	if len(toDelete) > 0 {
		if err := s.db.WithContext(ctx).
			Exec("UPDATE kb_document_chunks SET status = ? WHERE id IN ?", "superseded", toDelete).Error; err != nil {
			logger.Warnf("[KBUpdate] 标记 superseded 失败 doc=%d: %v", documentID, err)
		}
		result.Removed = len(toDelete)
	}

	newIdx := 0
	for _, c := range newChunks {
		h := contentHash(c)
		newIdx = newHashes[h]
		if _, existed := oldHashSet[h]; !existed {
			chunk := KBDocumentChunk{
				DocumentID:   documentID,
				ChunkIndex:   newIdx,
				ContentHash:  h,
				ChunkContent: c,
				Status:       "active",
			}
			if err := s.db.WithContext(ctx).Create(&chunk).Error; err != nil {
				logger.Warnf("[KBUpdate] 新增 chunk 失败 doc=%d idx=%d: %v", documentID, newIdx, err)
			}
			result.Added++
		} else {
			result.Unchanged++
		}
	}

	result.FinishedAt = time.Now()
	logger.Infof("[KBUpdate] 增量更新完成 doc=%d unchanged=%d added=%d removed=%d duration=%s",
		documentID, result.Unchanged, result.Added, result.Removed,
		result.FinishedAt.Sub(startedAt).Round(time.Millisecond))
	return result, nil
}

func contentHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func splitIntoChunks(content string, windowSize int) []string {
	if windowSize <= 0 {
		windowSize = 500
	}
	runes := []rune(content)
	if len(runes) == 0 {
		return []string{}
	}
	var chunks []string
	for i := 0; i < len(runes); i += windowSize {
		end := i + windowSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}
