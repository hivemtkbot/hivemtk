package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"marketing/internal/aiagent/llm"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
)

// MemorySystem 4 层记忆系统入口
// 对应 SYSTEM_AUDIT_REPORT_20260715_V3 P0-13
// L1 短期: 当前会话最近 N 条消息（DB 持久化 + 可选 Redis 加速）
// L2 长期: 客户档案、关键事实、对话摘要（PostgreSQL + 嵌入向量预留）
// L3 SOP 状态: SOP 流程级执行位置（与 sop_executions 同步）
// L4 业务: 订单/咨询/异议/意向等业务实体记忆
//
// P1-1 G5 增强（2026-07-17）：L2 长期记忆新增 pgvector 增强版（CustomerLongTermMemory）
//   - Remember/Recall 提供向量检索 + 重排序（importance + 时间衰减）
//   - 与原 L2SaveFact/L2ListFacts 并行，互不干扰
type MemorySystem struct {
	db           *gorm.DB
	embeddingSvc llm.EmbeddingServiceInterface
	mu           sync.Mutex
}

const (
	L1WindowSize = 10             // L1 保留消息数
	L1TTLHours   = 24 * time.Hour // L1 过期时间
	L4MaxPerCust = 500            // L4 每客户最大条数（防爆）
	defaultImp   = 5

	// P1-1 G5 L2 长期记忆 pgvector 相关常量
	longTermMemoryRecallMultiplier = 3                   // 粗召回倍数：limit * 3，避免重排序后错过重要记忆
	longTermMemoryMaxFetch         = 50                  // 单次召回最大条数
	longTermMemoryDecayDuration    = 30 * 24 * time.Hour // 30 天衰减为 0
)

var (
	memorySystemOnce sync.Once
	memorySystem     *MemorySystem
)

// GetMemorySystem 获取全局 4 层记忆系统
func GetMemorySystem() *MemorySystem {
	return memorySystem
}

// InitMemorySystem 初始化 4 层记忆系统
// 默认注入 llm.NewEmbeddingService()（本地 TEI 真实 bge-m3，1024 维）
// 测试场景可用 SetEmbeddingService 替换为 HashEmbeddingService
//
// 注意：测试模式下（IS_TEST_MODE=1）跳过 sync.Once 缓存，每次返回新实例，
// 避免测试间状态污染（不同测试用不同 DB 时全局缓存会导致数据写到错误 DB）。
func InitMemorySystem(db *gorm.DB) *MemorySystem {
	if os.Getenv("IS_TEST_MODE") == "1" {
		return &MemorySystem{
			db:           db,
			embeddingSvc: llm.NewEmbeddingService(),
		}
	}
	memorySystemOnce.Do(func() {
		memorySystem = &MemorySystem{
			db:           db,
			embeddingSvc: llm.NewEmbeddingService(),
		}
	})
	return memorySystem
}

// SetEmbeddingService 替换 Embedding 服务（用于测试注入 HashEmbeddingService）
// 非并发安全，应在初始化阶段调用
func (m *MemorySystem) SetEmbeddingService(svc llm.EmbeddingServiceInterface) {
	m.embeddingSvc = svc
}

// WithEmbeddingService 链式调用注入 Embedding 服务
func (m *MemorySystem) WithEmbeddingService(svc llm.EmbeddingServiceInterface) *MemorySystem {
	m.embeddingSvc = svc
	return m
}

// =================== L1 短期记忆 ===================

// L1Append 追加一条短期消息
func (m *MemorySystem) L1Append(ctx context.Context, sessionID, customerID, role, content string) error {
	if m.db == nil {
		return nil
	}
	exp := time.Now().Add(L1TTLHours)
	item := &model.MemoryItem{
		Layer:      model.MemoryLayerShortTerm,
		SessionID:  sessionID,
		CustomerID: customerID,
		ItemType:   "message",
		Role:       role,
		Content:    content,
		ExpiresAt:  &exp,
	}
	if err := m.db.WithContext(ctx).Create(item).Error; err != nil {
		return err
	}
	// 滑动窗口裁剪：保留最近 L1WindowSize 条
	m.l1Trim(ctx, sessionID)
	return nil
}

// L1List 获取会话最近 N 条消息
func (m *MemorySystem) L1List(ctx context.Context, sessionID string, limit int) ([]model.MemoryItem, error) {
	if m.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = L1WindowSize
	}
	var items []model.MemoryItem
	err := m.db.WithContext(ctx).
		Where("layer = ? AND session_id = ?", model.MemoryLayerShortTerm, sessionID).
		Order("created_at DESC").Limit(limit).
		Find(&items).Error
	return items, err
}

// L1Clear 清空某会话短期记忆
func (m *MemorySystem) L1Clear(ctx context.Context, sessionID string) error {
	if m.db == nil {
		return nil
	}
	return m.db.WithContext(ctx).
		Where("layer = ? AND session_id = ?", model.MemoryLayerShortTerm, sessionID).
		Delete(&model.MemoryItem{}).Error
}

// l1Trim 裁剪到 L1WindowSize
func (m *MemorySystem) l1Trim(ctx context.Context, sessionID string) {
	if m.db == nil {
		return
	}
	// 计算当前总数
	var count int64
	if err := m.db.WithContext(ctx).Model(&model.MemoryItem{}).
		Where("layer = ? AND session_id = ?", model.MemoryLayerShortTerm, sessionID).
		Count(&count).Error; err != nil {
		return
	}
	if count <= int64(L1WindowSize) {
		return
	}
	// 删除超出窗口的旧消息
	exceed := count - int64(L1WindowSize)
	var oldIDs []uint
	if err := m.db.WithContext(ctx).Model(&model.MemoryItem{}).
		Where("layer = ? AND session_id = ?", model.MemoryLayerShortTerm, sessionID).
		Order("created_at ASC").Limit(int(exceed)).
		Pluck("id", &oldIDs).Error; err != nil {
		return
	}
	if len(oldIDs) > 0 {
		m.db.WithContext(ctx).Where("id IN ?", oldIDs).Delete(&model.MemoryItem{})
	}
}

// =================== L2 长期记忆 ===================

// L2SaveFact 保存一条长期事实
func (m *MemorySystem) L2SaveFact(ctx context.Context, customerID, key, value string, importance int) error {
	if m.db == nil {
		return nil
	}
	if importance <= 0 || importance > 10 {
		importance = defaultImp
	}
	item := &model.MemoryItem{
		Layer:      model.MemoryLayerLongTerm,
		CustomerID: customerID,
		ItemType:   "fact:" + key,
		Content:    value,
		Importance: importance,
		Metadata:   model.JSONMap{"key": key},
	}
	return m.db.WithContext(ctx).Create(item).Error
}

// L2SaveSummary 保存长期摘要
func (m *MemorySystem) L2SaveSummary(ctx context.Context, customerID, summary string) error {
	if m.db == nil {
		return nil
	}
	item := &model.MemoryItem{
		Layer:      model.MemoryLayerLongTerm,
		CustomerID: customerID,
		ItemType:   "summary",
		Content:    summary,
		Importance: 8,
	}
	return m.db.WithContext(ctx).Create(item).Error
}

// L2ListFacts 获取客户的长期事实
func (m *MemorySystem) L2ListFacts(ctx context.Context, customerID string, limit int) ([]model.MemoryItem, error) {
	if m.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	var items []model.MemoryItem
	err := m.db.WithContext(ctx).
		Where("layer = ? AND customer_id = ? AND item_type LIKE ?", model.MemoryLayerLongTerm, customerID, "fact:%").
		Order("importance DESC, created_at DESC").Limit(limit).
		Find(&items).Error
	return items, err
}

// L2GetLatestSummary 获取客户最新长期摘要
func (m *MemorySystem) L2GetLatestSummary(ctx context.Context, customerID string) (string, error) {
	if m.db == nil {
		return "", nil
	}
	var item model.MemoryItem
	err := m.db.WithContext(ctx).
		Where("layer = ? AND customer_id = ? AND item_type = ?", model.MemoryLayerLongTerm, customerID, "summary").
		Order("created_at DESC").First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", err
	}
	return item.Content, nil
}

// =================== L3 SOP 状态 ===================

// L3SaveSOPState 保存 SOP 状态
func (m *MemorySystem) L3SaveSOPState(ctx context.Context, state *model.SOPStateMemory) error {
	if m.db == nil || state == nil {
		return nil
	}
	state.LastStepAt = time.Now()
	return m.db.WithContext(ctx).Save(state).Error
}

// L3GetSOPState 获取会话当前 SOP 状态
func (m *MemorySystem) L3GetSOPState(ctx context.Context, sessionID string) (*model.SOPStateMemory, error) {
	if m.db == nil {
		return nil, nil
	}
	var state model.SOPStateMemory
	err := m.db.WithContext(ctx).
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

// L3ListByCustomer 获取客户的所有 SOP 状态
func (m *MemorySystem) L3ListByCustomer(ctx context.Context, customerID string, limit int) ([]model.SOPStateMemory, error) {
	if m.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	var list []model.SOPStateMemory
	err := m.db.WithContext(ctx).
		Where("customer_id = ?", customerID).
		Order("updated_at DESC").Limit(limit).Find(&list).Error
	return list, err
}

// =================== L4 业务记忆 ===================

// L4Record 记录业务记忆
func (m *MemorySystem) L4Record(ctx context.Context, customerID, memoryType, content, relatedID string, importance int, metadata map[string]any) error {
	if m.db == nil {
		return nil
	}
	if importance <= 0 || importance > 10 {
		importance = defaultImp
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	// 限额裁剪：超过 L4MaxPerCust 删旧的
	var count int64
	m.db.WithContext(ctx).Model(&model.BusinessMemory{}).
		Where("customer_id = ?", customerID).Count(&count)
	if count >= int64(L4MaxPerCust) {
		exceed := count - int64(L4MaxPerCust) + 1
		var oldIDs []uint
		m.db.WithContext(ctx).Model(&model.BusinessMemory{}).
			Where("customer_id = ?", customerID).
			Order("importance ASC, created_at ASC").Limit(int(exceed)).
			Pluck("id", &oldIDs)
		if len(oldIDs) > 0 {
			m.db.WithContext(ctx).Where("id IN ?", oldIDs).Delete(&model.BusinessMemory{})
		}
	}

	meta := model.JSONMap{}
	if metadata != nil {
		for k, v := range metadata {
			meta[k] = v
		}
	}
	item := &model.BusinessMemory{
		CustomerID: customerID,
		MemoryType: memoryType,
		Content:    content,
		RelatedID:  relatedID,
		Importance: importance,
		Metadata:   meta,
	}
	return m.db.WithContext(ctx).Create(item).Error
}

// L4ListByCustomer 获取客户业务记忆
func (m *MemorySystem) L4ListByCustomer(ctx context.Context, customerID string, memoryType string, limit int) ([]model.BusinessMemory, error) {
	if m.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	q := m.db.WithContext(ctx).Model(&model.BusinessMemory{}).Where("customer_id = ?", customerID)
	if memoryType != "" {
		q = q.Where("memory_type = ?", memoryType)
	}
	var list []model.BusinessMemory
	err := q.Order("importance DESC, created_at DESC").Limit(limit).Find(&list).Error
	return list, err
}

// =================== 跨层聚合 ===================

// BuildFullContext 构造 4 层汇总上下文（用于 LLM 提示）
func (m *MemorySystem) BuildFullContext(ctx context.Context, sessionID, customerID string) (string, error) {
	if m.db == nil {
		return "", nil
	}
	var sb strings.Builder

	// L1 短期
	l1, _ := m.L1List(ctx, sessionID, L1WindowSize)
	if len(l1) > 0 {
		sb.WriteString("【L1 短期记忆（最近对话）】\n")
		// l1 是倒序的，需要反向
		for i := len(l1) - 1; i >= 0; i-- {
			role := "客户"
			if l1[i].Role == "ai" || l1[i].Role == "agent" {
				role = "我"
			}
			sb.WriteString(fmt.Sprintf("%s: %s\n", role, l1[i].Content))
		}
		sb.WriteString("\n")
	}

	// L2 长期
	if customerID != "" {
		if summary, _ := m.L2GetLatestSummary(ctx, customerID); summary != "" {
			sb.WriteString("【L2 长期摘要】\n")
			sb.WriteString(summary + "\n\n")
		}
		facts, _ := m.L2ListFacts(ctx, customerID, 20)
		if len(facts) > 0 {
			sb.WriteString("【L2 关键事实】\n")
			for _, f := range facts {
				sb.WriteString(fmt.Sprintf("- %s\n", f.Content))
			}
			sb.WriteString("\n")
		}
	}

	// L3 SOP 状态
	if sessionID != "" {
		state, _ := m.L3GetSOPState(ctx, sessionID)
		if state != nil {
			sb.WriteString("【L3 SOP 状态】\n")
			sb.WriteString(fmt.Sprintf("当前流程=%d, 节点=%s, 步骤=%d, 状态=%s\n\n",
				state.SOPID, state.CurrentNode, state.StepIndex, state.Status))
		}
	}

	// L4 业务
	if customerID != "" {
		biz, _ := m.L4ListByCustomer(ctx, customerID, "", 10)
		if len(biz) > 0 {
			sb.WriteString("【L4 业务记忆】\n")
			for _, b := range biz {
				sb.WriteString(fmt.Sprintf("[%s] %s\n", b.MemoryType, b.Content))
			}
		}
	}

	return sb.String(), nil
}

// SyncFromDialogueMemory 从老的 DialogueMemory 同步到 4 层结构
// 兼容层：保证重启后老数据也能被 4 层系统看到
func (m *MemorySystem) SyncFromDialogueMemory(ctx context.Context, mem *model.DialogueMemory) {
	if m.db == nil || mem == nil {
		return
	}
	if mem.CustomerID == "" {
		return
	}
	// L2 关键事实
	if len(mem.KeyFacts) > 0 {
		for k, v := range mem.KeyFacts {
			if s, ok := v.(string); ok && s != "" {
				m.L2SaveFact(ctx, mem.CustomerID, k, s, 7)
			}
		}
	}
	// L2 摘要
	if mem.Summary != "" {
		m.L2SaveSummary(ctx, mem.CustomerID, mem.Summary)
	}
	// L4 异议
	if len(mem.Objections) > 0 {
		objs, _ := json.Marshal(mem.Objections)
		m.L4Record(ctx, mem.CustomerID, "objection", string(objs), "", 7, nil)
	}
	// L4 购买意向
	if mem.PurchaseIntent != "" {
		m.L4Record(ctx, mem.CustomerID, "intent", "购买意向="+mem.PurchaseIntent, "", 8, nil)
	}
	// L4 偏好
	if mem.Budget != "" {
		m.L4Record(ctx, mem.CustomerID, "preference", "预算="+mem.Budget, "", 6, nil)
	}
	if mem.Demand != "" {
		m.L4Record(ctx, mem.CustomerID, "preference", "需求="+mem.Demand, "", 6, nil)
	}
	logger.Infof("[MemorySystem] 同步 DialogueMemory customer=%s 完成", mem.CustomerID)
}

// =================== P1-1 G5 L2 长期记忆（pgvector 增强） ===================

// longTermMemoryRow Recall 内部统一行结构（PG pgvector 召回结果）
type longTermMemoryRow struct {
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

// LongTermMemoryRecallResult Recall 结果
type LongTermMemoryRecallResult struct {
	Memory     *model.CustomerLongTermMemory
	Similarity float64 // 0-1, 越大越相似
	Score      float64 // 综合得分：similarity * 0.6 + importance_score * 0.3 + recency_score * 0.1
}

// Remember 记录一条长期记忆（自动 Embedding + 存储）
// 对应 PRD §5.2 P1-1 G5：MemorySystem.Remember(ctx, customerID, memType, content, importance)
// 验收：第一次对话客户说预算 5000，第二次对话 Recall 能主动返回该记忆
func (m *MemorySystem) Remember(ctx context.Context, customerID string, memType model.LongTermMemoryType, content string, importance int) (*model.CustomerLongTermMemory, error) {
	if m.db == nil {
		return nil, fmt.Errorf("memory system db not initialized")
	}
	if customerID == "" {
		return nil, fmt.Errorf("customer_id cannot be empty")
	}
	if content == "" {
		return nil, fmt.Errorf("content cannot be empty")
	}
	if m.embeddingSvc == nil {
		return nil, fmt.Errorf("embedding service not configured")
	}
	if importance <= 0 || importance > 10 {
		importance = defaultImp
	}
	switch memType {
	case model.LongTermMemoryPreference, model.LongTermMemoryHabit,
		model.LongTermMemoryFeedback, model.LongTermMemoryEvent, model.LongTermMemoryFact:
		// ok
	default:
		return nil, fmt.Errorf("invalid memory_type: %s", memType)
	}

	cfg := m.embeddingSvc.DefaultConfig()
	vec, err := m.embeddingSvc.EmbedOne(ctx, cfg, content)
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}

	item := &model.CustomerLongTermMemory{
		CustomerID: customerID,
		MemoryType: memType,
		Content:    content,
		Importance: importance,
		Source:     model.LongTermMemorySourceConversation,
		Embedding:  embeddingToString(vec),
		Metadata:   model.JSONMap{},
	}
	if err := m.db.WithContext(ctx).Create(item).Error; err != nil {
		return nil, fmt.Errorf("save long term memory: %w", err)
	}
	return item, nil
}

// RememberWithSource 记录长期记忆（带来源 + 元信息）
func (m *MemorySystem) RememberWithSource(ctx context.Context, customerID string, memType model.LongTermMemoryType, content string, importance int, source model.LongTermMemorySource, metadata map[string]any) (*model.CustomerLongTermMemory, error) {
	if m.db == nil {
		return nil, fmt.Errorf("memory system db not initialized")
	}
	if m.embeddingSvc == nil {
		return nil, fmt.Errorf("embedding service not configured")
	}
	item, err := m.Remember(ctx, customerID, memType, content, importance)
	if err != nil {
		return nil, err
	}
	item.Source = source
	if metadata != nil {
		meta := model.JSONMap{}
		for k, v := range metadata {
			meta[k] = v
		}
		item.Metadata = meta
	}
	if err := m.db.WithContext(ctx).Save(item).Error; err != nil {
		return nil, fmt.Errorf("update long term memory meta: %w", err)
	}
	return item, nil
}

// Recall 召回与 query 最相关的长期记忆
// 对应 PRD §5.2 P1-1 G5：MemorySystem.Recall(ctx, customerID, query, limit)
// 算法：向量检索（PG pgvector）+ 重排序（importance + 时间衰减）
//   - PG 环境：使用 pgvector 索引召回
//   - 无 pgvector（如未初始化 embedding）：降级为扫表 + 内存计算余弦相似度 + 重排序
func (m *MemorySystem) Recall(ctx context.Context, customerID, query string, limit int) ([]LongTermMemoryRecallResult, error) {
	if m.db == nil {
		return nil, fmt.Errorf("memory system db not initialized")
	}
	if customerID == "" {
		return nil, fmt.Errorf("customer_id cannot be empty")
	}
	if limit <= 0 {
		limit = 5
	}
	if m.embeddingSvc == nil {
		return nil, fmt.Errorf("embedding service not configured")
	}

	cfg := m.embeddingSvc.DefaultConfig()
	queryVec, err := m.embeddingSvc.EmbedOne(ctx, cfg, query)
	if err != nil {
		return nil, fmt.Errorf("query embedding failed: %w", err)
	}

	dialect := m.db.Dialector.Name()
	if dialect == "postgres" {
		return m.recallPostgres(ctx, customerID, queryVec, limit)
	}
	return m.recallFallback(ctx, customerID, queryVec, limit)
}

// recallPostgres 使用 pgvector 检索（生产路径）
//   - 粗召回：top-K * 3（避免重排序后错过重要记忆）
//   - 重排序：similarity * 0.6 + importance_score * 0.3 + recency_score * 0.1
func (m *MemorySystem) recallPostgres(ctx context.Context, customerID string, queryVec []float32, limit int) ([]LongTermMemoryRecallResult, error) {
	fetchN := limit * longTermMemoryRecallMultiplier
	if fetchN > longTermMemoryMaxFetch {
		fetchN = longTermMemoryMaxFetch
	}
	// pgvector 的 ?::vector 期望 text 通道传入 '[v1,v2,...]' 格式。
	// 若传入 []byte，PG 驱动会按 bytea 发送，::vector 无法从 bytea 转换，
	// 报 "invalid input syntax for type vector"。
	// 因此必须使用 string 形式（embeddingToString 输出 JSON 数组文本）。
	queryVecStr := embeddingToString(queryVec)
	sql := `
		SELECT id, customer_id, memory_type, content, importance, source, metadata, created_at, expires_at,
		       1 - (embedding <=> ?::vector) as similarity
		FROM customer_long_term_memory
		WHERE customer_id = ? AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY embedding <=> ?::vector
		LIMIT ?
	`
	var rows []longTermMemoryRow
	if err := m.db.WithContext(ctx).Raw(sql, queryVecStr, customerID, queryVecStr, fetchN).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("pgvector search: %w", err)
	}
	return m.rerank(rows, limit), nil
}

// recallFallback pgvector 缺失降级路径：内存计算余弦相似度
// 不依赖 pgvector，仅在 embedding 服务未初始化或 pgvector 扩展不可用时使用
func (m *MemorySystem) recallFallback(ctx context.Context, customerID string, queryVec []float32, limit int) ([]LongTermMemoryRecallResult, error) {
	var items []model.CustomerLongTermMemory
	if err := m.db.WithContext(ctx).
		Where("customer_id = ? AND (expires_at IS NULL OR expires_at > ?)", customerID, time.Now()).
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("fallback fetch: %w", err)
	}

	rows := make([]longTermMemoryRow, 0, len(items))
	for _, it := range items {
		vec := bytesToFloat32Slice([]byte(it.Embedding))
		sim := cosineSimilarity(queryVec, vec)
		meta := ""
		if it.Metadata != nil {
			if b, err := json.Marshal(it.Metadata); err == nil {
				meta = string(b)
			}
		}
		rows = append(rows, longTermMemoryRow{
			ID:         it.ID,
			CustomerID: it.CustomerID,
			MemoryType: string(it.MemoryType),
			Content:    it.Content,
			Importance: it.Importance,
			Source:     string(it.Source),
			Metadata:   meta,
			CreatedAt:  it.CreatedAt,
			ExpiresAt:  it.ExpiresAt,
			Similarity: sim,
		})
	}
	return m.rerank(rows, limit), nil
}

// rerank 重排序：综合得分 = similarity * 0.6 + importance_score * 0.3 + recency_score * 0.1
//   - importance_score = importance / 10
//   - recency_score = 1 - (now - created_at) / 30d（30 天衰减为 0，clamp 到 [0,1]）
func (m *MemorySystem) rerank(rows []longTermMemoryRow, limit int) []LongTermMemoryRecallResult {
	now := time.Now()
	results := make([]LongTermMemoryRecallResult, 0, len(rows))
	for _, r := range rows {
		importanceScore := float64(r.Importance) / 10.0
		recencyScore := 1.0 - float64(now.Sub(r.CreatedAt))/float64(longTermMemoryDecayDuration)
		if recencyScore < 0 {
			recencyScore = 0
		}
		if recencyScore > 1 {
			recencyScore = 1
		}
		score := r.Similarity*0.6 + importanceScore*0.3 + recencyScore*0.1

		var meta model.JSONMap
		if r.Metadata != "" {
			_ = json.Unmarshal([]byte(r.Metadata), &meta)
		}
		if meta == nil {
			meta = model.JSONMap{}
		}
		results = append(results, LongTermMemoryRecallResult{
			Memory: &model.CustomerLongTermMemory{
				ID:         r.ID,
				CustomerID: r.CustomerID,
				MemoryType: model.LongTermMemoryType(r.MemoryType),
				Content:    r.Content,
				Importance: r.Importance,
				Source:     model.LongTermMemorySource(r.Source),
				Metadata:   meta,
				CreatedAt:  r.CreatedAt,
				ExpiresAt:  r.ExpiresAt,
			},
			Similarity: r.Similarity,
			Score:      score,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

// ListLongTermMemories 列出客户长期记忆（按 importance + created_at 降序，不走向量检索）
// 用于 UI 展示 / 调试
func (m *MemorySystem) ListLongTermMemories(ctx context.Context, customerID string, memType string, limit int) ([]model.CustomerLongTermMemory, error) {
	if m.db == nil {
		return nil, nil
	}
	if customerID == "" {
		return nil, fmt.Errorf("customer_id cannot be empty")
	}
	if limit <= 0 {
		limit = 50
	}
	q := m.db.WithContext(ctx).Model(&model.CustomerLongTermMemory{}).Where("customer_id = ?", customerID)
	if memType != "" {
		q = q.Where("memory_type = ?", memType)
	}
	var list []model.CustomerLongTermMemory
	err := q.Order("importance DESC, created_at DESC").Limit(limit).Find(&list).Error
	return list, err
}

// DeleteLongTermMemory 删除指定 ID 的长期记忆
func (m *MemorySystem) DeleteLongTermMemory(ctx context.Context, id uint64) error {
	if m.db == nil {
		return nil
	}
	return m.db.WithContext(ctx).Delete(&model.CustomerLongTermMemory{}, id).Error
}

// float32SliceToBytes 将 float32 切片序列化为 []byte（JSON 格式，pgvector 兼容）
// 与 rag_retrieval 包中同名函数保持一致行为：pgvector 接受 JSON 数组格式向量
func float32SliceToBytes(vec []float32) []byte {
	data, _ := json.Marshal(vec)
	return data
}

// bytesToFloat32Slice 反序列化 float32 切片
func bytesToFloat32Slice(data []byte) []float32 {
	if len(data) == 0 {
		return nil
	}
	var vec []float32
	_ = json.Unmarshal(data, &vec)
	return vec
}

// embeddingToString 将 float32 切片序列化为 pgvector 文本格式 '[v1,v2,...]'
// 用于 model.CustomerLongTermMemory.Embedding（string 字段），GORM 通过 text 通道
// 传给 PostgreSQL 时 pgvector 可直接解析为 vector(1024)。
// 与 float32SliceToBytes 的区别：后者输出 JSON 字符串的 []byte 表示（用于 bytea 通道），
// 此处输出纯字符串供 SQL 参数绑定使用。
func embeddingToString(vec []float32) string {
	if len(vec) == 0 {
		return ""
	}
	// 复用 float32SliceToBytes 的 JSON 序列化结果
	return string(float32SliceToBytes(vec))
}

// cosineSimilarity 计算两个向量的余弦相似度（pgvector 降级路径用）
// 返回值范围 [-1, 1]，相同方向为 1，正交为 0，相反为 -1
func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		af := float64(a[i])
		bf := float64(b[i])
		dot += af * bf
		na += af * af
		nb += bf * bf
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
