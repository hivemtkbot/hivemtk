package repository

// memory_repository.go 4 层记忆系统仓储（L1 短期 / L2 长期 / L3 SOP 状态 / L4 业务）
//
// 五层架构归属: L4 数据访问层
// 设计依据: service/memory_system.go 历史直连 DB 操作下沉
//
// 覆盖 model:
//   - MemoryItem (L1 短期 / L2 长期事实&摘要)
//   - SOPStateMemory (L3 SOP 状态)
//   - BusinessMemory (L4 业务记忆)
//   - CustomerLongTermMemory (L2 pgvector 增强版)

import (
	"context"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
)

// LongTermMemoryVectorRow pgvector 召回 SQL 扫描行
// 与 service.longTermMemoryRow 等价，由 repository 持有以避免 service 直接访问 DB
type LongTermMemoryVectorRow struct {
	ID         uint64
	CustomerID string
	MemoryType string
	Content    string
	Importance int
	Source     string
	Metadata   string
	CreatedAt  time.Time
	ExpiresAt  *time.Time
	Similarity float64
}

// MemoryRepository 4 层记忆系统仓储接口
type MemoryRepository interface {
	// L1 短期记忆
	CreateMemoryItem(ctx context.Context, item *model.MemoryItem) error
	ListShortTermMemoryBySession(ctx context.Context, sessionID string, limit int) ([]model.MemoryItem, error)
	DeleteShortTermMemoryBySession(ctx context.Context, sessionID string) error
	CountShortTermMemoryBySession(ctx context.Context, sessionID string) (int64, error)
	PluckOldestShortTermMemoryIDs(ctx context.Context, sessionID string, limit int) ([]uint, error)
	DeleteMemoryItemsByIDs(ctx context.Context, ids []uint) error

	// L2 长期事实 / 摘要（MemoryItem 表）
	ListFacts(ctx context.Context, customerID string, limit int) ([]model.MemoryItem, error)
	GetLatestSummary(ctx context.Context, customerID string) (*model.MemoryItem, error)

	// L3 SOP 状态
	SaveSOPState(ctx context.Context, state *model.SOPStateMemory) error
	GetSOPStateBySession(ctx context.Context, sessionID string) (*model.SOPStateMemory, error)
	ListSOPStatesByCustomer(ctx context.Context, customerID string, limit int) ([]model.SOPStateMemory, error)

	// L4 业务记忆
	CountBusinessMemoriesByCustomer(ctx context.Context, customerID string) (int64, error)
	PluckOldestBusinessMemoryIDs(ctx context.Context, customerID string, limit int) ([]uint, error)
	DeleteBusinessMemoriesByIDs(ctx context.Context, ids []uint) error
	CreateBusinessMemory(ctx context.Context, item *model.BusinessMemory) error
	ListBusinessMemories(ctx context.Context, customerID, memoryType string, limit int) ([]model.BusinessMemory, error)

	// L2 长期记忆 pgvector 增强版
	CreateLongTermMemory(ctx context.Context, item *model.CustomerLongTermMemory) error
	SaveLongTermMemory(ctx context.Context, item *model.CustomerLongTermMemory) error
	SearchLongTermMemoriesByVector(ctx context.Context, queryVecStr, customerID string, fetchN int) ([]LongTermMemoryVectorRow, error)
	ListLongTermMemoriesForFallback(ctx context.Context, customerID string, now time.Time) ([]model.CustomerLongTermMemory, error)
	ListLongTermMemories(ctx context.Context, customerID, memType string, limit int) ([]model.CustomerLongTermMemory, error)
	DeleteLongTermMemoryByID(ctx context.Context, id uint64) error

	// DB 元信息
	DialectName(ctx context.Context) string
}

// memoryRepository 实现 MemoryRepository
type memoryRepository struct {
	db *gorm.DB
}

// NewMemoryRepository 构造（无参，内部取库句柄）
func NewMemoryRepository() MemoryRepository {
	return &memoryRepository{db: _db.GetDB()}
}

// NewMemoryRepositoryWithDB 创建指定数据库连接的 MemoryRepository 实例（用于测试）
func NewMemoryRepositoryWithDB(db *gorm.DB) MemoryRepository {
	return &memoryRepository{db: db}
}

// =================== L1 短期记忆 ===================

// CreateMemoryItem 创建 MemoryItem（L1 短期消息 / L2 事实摘要通用）
func (r *memoryRepository) CreateMemoryItem(ctx context.Context, item *model.MemoryItem) error {
	return r.db.WithContext(ctx).Create(item).Error
}

// ListShortTermMemoryBySession 按会话拉取 L1 短期消息（按 created_at DESC）
func (r *memoryRepository) ListShortTermMemoryBySession(ctx context.Context, sessionID string, limit int) ([]model.MemoryItem, error) {
	var items []model.MemoryItem
	err := r.db.WithContext(ctx).
		Where("layer = ? AND session_id = ?", model.MemoryLayerShortTerm, sessionID).
		Order("created_at DESC").Limit(limit).
		Find(&items).Error
	return items, err
}

// DeleteShortTermMemoryBySession 清空会话的 L1 短期记忆
func (r *memoryRepository) DeleteShortTermMemoryBySession(ctx context.Context, sessionID string) error {
	return r.db.WithContext(ctx).
		Where("layer = ? AND session_id = ?", model.MemoryLayerShortTerm, sessionID).
		Delete(&model.MemoryItem{}).Error
}

// CountShortTermMemoryBySession 统计会话 L1 短期消息数
func (r *memoryRepository) CountShortTermMemoryBySession(ctx context.Context, sessionID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.MemoryItem{}).
		Where("layer = ? AND session_id = ?", model.MemoryLayerShortTerm, sessionID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// PluckOldestShortTermMemoryIDs 取会话最早的 N 条 L1 短期消息 ID（按 created_at ASC）
// 用于滑动窗口裁剪
func (r *memoryRepository) PluckOldestShortTermMemoryIDs(ctx context.Context, sessionID string, limit int) ([]uint, error) {
	var ids []uint
	if err := r.db.WithContext(ctx).Model(&model.MemoryItem{}).
		Where("layer = ? AND session_id = ?", model.MemoryLayerShortTerm, sessionID).
		Order("created_at ASC").Limit(limit).
		Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// DeleteMemoryItemsByIDs 按 ID 批量删除 MemoryItem
func (r *memoryRepository) DeleteMemoryItemsByIDs(ctx context.Context, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&model.MemoryItem{}).Error
}

// =================== L2 长期事实 / 摘要 ===================

// ListFacts 列出客户长期事实（item_type LIKE 'fact:%'，按 importance DESC, created_at DESC）
func (r *memoryRepository) ListFacts(ctx context.Context, customerID string, limit int) ([]model.MemoryItem, error) {
	var items []model.MemoryItem
	err := r.db.WithContext(ctx).
		Where("layer = ? AND customer_id = ? AND item_type LIKE ?", model.MemoryLayerLongTerm, customerID, "fact:%").
		Order("importance DESC, created_at DESC").Limit(limit).
		Find(&items).Error
	return items, err
}

// GetLatestSummary 取客户最新长期摘要（item_type = 'summary'）
// 未找到时返回 (nil, nil)（与 service 原行为一致）
func (r *memoryRepository) GetLatestSummary(ctx context.Context, customerID string) (*model.MemoryItem, error) {
	var item model.MemoryItem
	err := r.db.WithContext(ctx).
		Where("layer = ? AND customer_id = ? AND item_type = ?", model.MemoryLayerLongTerm, customerID, "summary").
		Order("created_at DESC").First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

// =================== L3 SOP 状态 ===================

// SaveSOPState 保存 SOP 状态（upsert by ID）
func (r *memoryRepository) SaveSOPState(ctx context.Context, state *model.SOPStateMemory) error {
	return r.db.WithContext(ctx).Save(state).Error
}

// GetSOPStateBySession 按 session 取最新 SOP 状态
// 未找到时返回 (nil, nil)
func (r *memoryRepository) GetSOPStateBySession(ctx context.Context, sessionID string) (*model.SOPStateMemory, error) {
	var state model.SOPStateMemory
	err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("updated_at DESC").First(&state).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &state, nil
}

// ListSOPStatesByCustomer 列出客户 SOP 状态（按 updated_at DESC）
func (r *memoryRepository) ListSOPStatesByCustomer(ctx context.Context, customerID string, limit int) ([]model.SOPStateMemory, error) {
	var list []model.SOPStateMemory
	err := r.db.WithContext(ctx).
		Where("customer_id = ?", customerID).
		Order("updated_at DESC").Limit(limit).Find(&list).Error
	return list, err
}

// =================== L4 业务记忆 ===================

// CountBusinessMemoriesByCustomer 统计客户业务记忆数（用于限额裁剪）
// 注意：与原实现一致，错误被静默忽略（返回 0），调用方按计数判断是否裁剪
func (r *memoryRepository) CountBusinessMemoriesByCustomer(ctx context.Context, customerID string) (int64, error) {
	var count int64
	r.db.WithContext(ctx).Model(&model.BusinessMemory{}).
		Where("customer_id = ?", customerID).Count(&count)
	return count, nil
}

// PluckOldestBusinessMemoryIDs 取客户最旧 N 条业务记忆 ID
// 按 importance ASC, created_at ASC（重要性低的、旧的优先删）
// 注意：与原实现一致，错误被静默忽略（返回 nil）
func (r *memoryRepository) PluckOldestBusinessMemoryIDs(ctx context.Context, customerID string, limit int) ([]uint, error) {
	var ids []uint
	r.db.WithContext(ctx).Model(&model.BusinessMemory{}).
		Where("customer_id = ?", customerID).
		Order("importance ASC, created_at ASC").Limit(limit).
		Pluck("id", &ids)
	return ids, nil
}

// DeleteBusinessMemoriesByIDs 按 ID 批量删除 BusinessMemory
func (r *memoryRepository) DeleteBusinessMemoriesByIDs(ctx context.Context, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&model.BusinessMemory{}).Error
}

// CreateBusinessMemory 创建业务记忆
func (r *memoryRepository) CreateBusinessMemory(ctx context.Context, item *model.BusinessMemory) error {
	return r.db.WithContext(ctx).Create(item).Error
}

// ListBusinessMemories 列出客户业务记忆（按 importance DESC, created_at DESC）
// memoryType 为空时不过滤
func (r *memoryRepository) ListBusinessMemories(ctx context.Context, customerID, memoryType string, limit int) ([]model.BusinessMemory, error) {
	q := r.db.WithContext(ctx).Model(&model.BusinessMemory{}).Where("customer_id = ?", customerID)
	if memoryType != "" {
		q = q.Where("memory_type = ?", memoryType)
	}
	var list []model.BusinessMemory
	err := q.Order("importance DESC, created_at DESC").Limit(limit).Find(&list).Error
	return list, err
}

// =================== L2 长期记忆 pgvector 增强版 ===================

// CreateLongTermMemory 创建长期记忆（带 embedding）
func (r *memoryRepository) CreateLongTermMemory(ctx context.Context, item *model.CustomerLongTermMemory) error {
	return r.db.WithContext(ctx).Create(item).Error
}

// SaveLongTermMemory 保存长期记忆（更新 source / metadata 等字段）
func (r *memoryRepository) SaveLongTermMemory(ctx context.Context, item *model.CustomerLongTermMemory) error {
	return r.db.WithContext(ctx).Save(item).Error
}

// SearchLongTermMemoriesByVector pgvector 召回（生产路径）
//
// SQL 参数顺序与原 service.recallPostgres 完全一致：
//
//	SELECT ..., 1 - (embedding <=> ?::vector) as similarity
//	FROM customer_long_term_memory
//	WHERE customer_id = ? AND (expires_at IS NULL OR expires_at > NOW())
//	ORDER BY embedding <=> ?::vector
//	LIMIT ?
//
// queryVecStr 必须为 pgvector 文本格式 '[v1,v2,...]'，
// 由 service 层通过 embeddingToString 序列化后传入。
func (r *memoryRepository) SearchLongTermMemoriesByVector(ctx context.Context, queryVecStr, customerID string, fetchN int) ([]LongTermMemoryVectorRow, error) {
	sql := `
		SELECT id, customer_id, memory_type, content, importance, source, metadata, created_at, expires_at,
		       1 - (embedding <=> ?::vector) as similarity
		FROM customer_long_term_memory
		WHERE customer_id = ? AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY embedding <=> ?::vector
		LIMIT ?
	`
	var rows []LongTermMemoryVectorRow
	if err := r.db.WithContext(ctx).Raw(sql, queryVecStr, customerID, queryVecStr, fetchN).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ListLongTermMemoriesForFallback pgvector 降级路径：按 customer_id 拉全部有效记忆
// 用于内存计算余弦相似度
func (r *memoryRepository) ListLongTermMemoriesForFallback(ctx context.Context, customerID string, now time.Time) ([]model.CustomerLongTermMemory, error) {
	var items []model.CustomerLongTermMemory
	if err := r.db.WithContext(ctx).
		Where("customer_id = ? AND (expires_at IS NULL OR expires_at > ?)", customerID, now).
		Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// ListLongTermMemories 列出客户长期记忆（按 importance DESC, created_at DESC，不走向量检索）
// memType 为空时不过滤
func (r *memoryRepository) ListLongTermMemories(ctx context.Context, customerID, memType string, limit int) ([]model.CustomerLongTermMemory, error) {
	q := r.db.WithContext(ctx).Model(&model.CustomerLongTermMemory{}).Where("customer_id = ?", customerID)
	if memType != "" {
		q = q.Where("memory_type = ?", memType)
	}
	var list []model.CustomerLongTermMemory
	err := q.Order("importance DESC, created_at DESC").Limit(limit).Find(&list).Error
	return list, err
}

// DeleteLongTermMemoryByID 按 ID 删除长期记忆
func (r *memoryRepository) DeleteLongTermMemoryByID(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&model.CustomerLongTermMemory{}, id).Error
}

// =================== DB 元信息 ===================

// DialectName 返回当前 DB dialect 名（如 "postgres" / "sqlite"）
// 用于 service 判断是否走 pgvector 召回路径
func (r *memoryRepository) DialectName(ctx context.Context) string {
	return r.db.Dialector.Name()
}
