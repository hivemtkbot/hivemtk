package repository

// faq_entry.go FAQ 知识库 Repository
//
// 五层架构归属: L5 数据访问层
// 设计依据: 2026-07-31 AI 智能体性能优化 (T2)
//   - Layer1 路由依赖 FAQ 快速匹配 (零 LLM, <100ms)
//   - MatchByKeyword: 基于中文分词 + 关键词包含打分
//   - 未来可扩展: pgvector 全文检索 + Embedding 向量召回
//
// 方法:
//   - Create           新增 FAQ
//   - GetByID          按 ID 查询
//   - ListEnabled      查询所有启用的 FAQ (用于内存缓存 warmup)
//   - MatchByKeyword   关键词匹配 (Layer1 核心 API)
//   - IncrementHitCount 命中次数 +1 (用于优化排序 + 报表)

import (
	"context"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"marketing/internal/model"

	"gorm.io/gorm"
)

type FAQRepository struct {
	db *gorm.DB
}

func NewFAQRepository(db *gorm.DB) *FAQRepository {
	return &FAQRepository{db: db}
}

// Create 新增 FAQ
//
// 注意: GORM v2 对 bool 零值 (false) 不会写入,会使用 column default。
// 此处用 Select("*") 强制全字段 INSERT,确保 false 也能正确落库。
func (r *FAQRepository) Create(ctx context.Context, entry *model.FAQEntry) error {
	return r.db.WithContext(ctx).Select("*").Create(entry).Error
}

// GetByID 按 ID 查询
func (r *FAQRepository) GetByID(ctx context.Context, id uint) (*model.FAQEntry, error) {
	var entry model.FAQEntry
	if err := r.db.WithContext(ctx).First(&entry, id).Error; err != nil {
		return nil, err
	}
	return &entry, nil
}

// ListEnabled 查询所有启用的 FAQ
func (r *FAQRepository) ListEnabled(ctx context.Context, limit int) ([]model.FAQEntry, error) {
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}
	var entries []model.FAQEntry
	err := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("hit_count DESC, confidence DESC, id ASC").
		Limit(limit).
		Find(&entries).Error
	return entries, err
}

// MatchByKeyword 关键词匹配 (Layer1 核心 API)
//
// 策略:
//  1. 取所有 enabled FAQ (数据量 < 1000 时, 全量内存打分)
//  2. 中文按字符 bigram 切词 + 关键词包含打分
//  3. 排序: score * (baseConfidence + log(hit_count+1)) 降序
//  4. 返回 top K
//
// 后续可扩展: pgvector 全文检索 / Embedding 向量召回
func (r *FAQRepository) MatchByKeyword(ctx context.Context, msg string, topK int) ([]model.FAQEntry, error) {
	if msg == "" {
		return nil, nil
	}
	all, err := r.ListEnabled(ctx, 5000)
	if err != nil {
		return nil, err
	}
	return r.scoreAndRank(ctx, all, msg, topK)
}

// MatchByIDs 按 ID 集合匹配 (2026-07-31 P1-A: 智能体绑定 FAQ 范围)
//
// agent 绑定了 FAQ 时, 仅在绑定的 IDs 内匹配; 绑定为空 = 全局共享
func (r *FAQRepository) MatchByIDs(ctx context.Context, msg string, ids []string, topK int) ([]model.FAQEntry, error) {
	if msg == "" || len(ids) == 0 {
		return nil, nil
	}
	var entries []model.FAQEntry
	err := r.db.WithContext(ctx).
		Where("enabled = ? AND id IN ?", true, ids).
		Order("hit_count DESC, confidence DESC, id ASC").
		Limit(5000).
		Find(&entries).Error
	if err != nil {
		return nil, err
	}
	return r.scoreAndRank(ctx, entries, msg, topK)
}

// scoreAndRank 内部打分+排序 (MatchByKeyword / MatchByIDs 共用)
func (r *FAQRepository) scoreAndRank(ctx context.Context, all []model.FAQEntry, msg string, topK int) ([]model.FAQEntry, error) {
	msgTokens := tokenize(msg)
	if len(msgTokens) == 0 {
		return nil, nil
	}
	type scored struct {
		entry model.FAQEntry
		score float64
	}
	var results []scored
	for i := range all {
		e := all[i]
		if e.Enabled == nil || !*e.Enabled {
			continue
		}
		faqTokens := tokenize(e.Question + " " + e.Answer)
		score := jaccardSimilarity(msgTokens, faqTokens)
		// 关键词精确包含加权
		for _, kw := range e.Keywords {
			if kw != "" && strings.Contains(msg, kw) {
				score += 0.3
			}
		}
		// 基础置信度 + 命中次数加权
		score = score * (e.Confidence + 0.1*logPlus(e.HitCount))
		if score < 0.2 {
			continue
		}
		results = append(results, scored{entry: e, score: score})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})
	if topK <= 0 || topK > len(results) {
		topK = len(results)
	}
	out := make([]model.FAQEntry, 0, topK)
	for i := 0; i < topK; i++ {
		results[i].entry.Confidence = results[i].score
		out = append(out, results[i].entry)
	}
	return out, nil
}

// ListWithFilter 分页+过滤 (前端 FAQ 管理页面使用)
func (r *FAQRepository) ListWithFilter(ctx context.Context, filter FAQFilter) ([]model.FAQEntry, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.FAQEntry{})
	if filter.Keyword != "" {
		kw := "%" + filter.Keyword + "%"
		q = q.Where("question LIKE ? OR answer LIKE ?", kw, kw)
	}
	if filter.Category != "" {
		q = q.Where("category = ?", filter.Category)
	}
	if filter.Intent != "" {
		q = q.Where("intent = ?", filter.Intent)
	}
	if filter.Enabled != nil {
		q = q.Where("enabled = ?", *filter.Enabled)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var entries []model.FAQEntry
	offset := (filter.Page - 1) * filter.PageSize
	if offset < 0 {
		offset = 0
	}
	err := q.Order("hit_count DESC, confidence DESC, id ASC").
		Offset(offset).Limit(filter.PageSize).
		Find(&entries).Error
	return entries, total, err
}

// Update 更新 FAQ
func (r *FAQRepository) Update(ctx context.Context, id uint, entry *model.FAQEntry) error {
	return r.db.WithContext(ctx).Model(&model.FAQEntry{}).
		Where("id = ?", id).
		Select("*").Updates(entry).Error
}

// Delete 删除 FAQ
func (r *FAQRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.FAQEntry{}).Error
}

// FAQFilter FAQ 查询过滤器 (前端管理页面)
type FAQFilter struct {
	Keyword  string
	Category string
	Intent   string
	Enabled  *bool
	Page     int
	PageSize int
}

// IncrementHitCount 命中次数 +1
func (r *FAQRepository) IncrementHitCount(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).
		Model(&model.FAQEntry{}).
		Where("id = ?", id).
		UpdateColumns(map[string]any{
			"hit_count":   gorm.Expr("hit_count + 1"),
			"last_hit_at": gorm.Expr("NOW()"),
		}).
		Error
}

// DecayQuality 衰减指定 FAQ 的 QualityScore (B-021: 质量衰减)
//
// 入参:
//   - id:    要衰减的 FAQ ID
//   - decay: 衰减量, 范围 [0, 1], 调用方负责边界 (repo 不做下限保护以保持纯函数语义)
//
// 行为:
//   - 用 GREATEST(quality_score - decay, 0) 保证不会衰减到负数
//   - 同一事务可多次调用叠加衰减
func (r *FAQRepository) DecayQuality(ctx context.Context, id uint, decay float64) error {
	if id == 0 || decay <= 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&model.FAQEntry{}).
		Where("id = ?", id).
		UpdateColumn("quality_score", gorm.Expr("GREATEST(quality_score - ?, 0)", decay)).
		Error
}

// ListDecayCandidates 查询符合衰减条件的 FAQ (B-021)
//
// 条件:
//   - hit_count < 5 (低命中)
//   - last_hit_at 非空 且 早于 cutoff (超过 7 天未被命中)
//
// cutoff 建议为 now-7d, 调用方注入
func (r *FAQRepository) ListDecayCandidates(ctx context.Context, cutoff time.Time, limit int) ([]model.FAQEntry, error) {
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}
	var entries []model.FAQEntry
	err := r.db.WithContext(ctx).
		Where("hit_count < ? AND last_hit_at IS NOT NULL AND last_hit_at < ?", 5, cutoff).
		Order("last_hit_at ASC").
		Limit(limit).
		Find(&entries).Error
	return entries, err
}

// IncrementNegativeHit 用户负反馈 +1 (B-021: 负反馈快速降权)
func (r *FAQRepository) IncrementNegativeHit(ctx context.Context, id uint) error {
	if id == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&model.FAQEntry{}).
		Where("id = ?", id).
		UpdateColumn("negative_hit_count", gorm.Expr("negative_hit_count + 1")).
		Error
}

// tokenize 中文 bigram 切词 (简化版, 无 jieba 依赖)
//
// 输入: "韵达发货吗" -> ["韵达", "达发", "发货", "货吗"]
// 输入: "abc def" -> ["abc", "def"]
func tokenize(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return nil
	}
	runes := []rune(s)
	tokens := make([]string, 0, len(runes))
	// 英文按空格切
	if !hasChinese(s) {
		for _, w := range strings.Fields(s) {
			if utf8.RuneCountInString(w) >= 2 {
				tokens = append(tokens, w)
			}
		}
		return tokens
	}
	// 中文 bigram
	for i := 0; i < len(runes)-1; i++ {
		if isCJK(runes[i]) && isCJK(runes[i+1]) {
			tokens = append(tokens, string(runes[i:i+2]))
		}
	}
	// 单字也算
	for _, r := range runes {
		if isCJK(r) {
			tokens = append(tokens, string(r))
		}
	}
	return tokens
}

func hasChinese(s string) bool {
	for _, r := range s {
		if isCJK(r) {
			return true
		}
	}
	return false
}

func isCJK(r rune) bool {
	return r >= 0x4E00 && r <= 0x9FFF
}

// jaccardSimilarity Jaccard 相似度
func jaccardSimilarity(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	setA := make(map[string]struct{}, len(a))
	for _, s := range a {
		setA[s] = struct{}{}
	}
	setB := make(map[string]struct{}, len(b))
	for _, s := range b {
		setB[s] = struct{}{}
	}
	inter := 0
	for k := range setA {
		if _, ok := setB[k]; ok {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// logPlus 自然对数平滑 (避免负值, 让 hit_count=0 时返回 0)
func logPlus(n int64) float64 {
	if n <= 0 {
		return 0
	}
	x := float64(n)
	// 简化: 使用近似 ln(x+1)/ln(10)
	// 避免引入 math 包的大开销
	// ln(1) = 0, ln(10) ≈ 2.3, ln(100) ≈ 4.6
	if x < 1 {
		return 0
	}
	if x < 10 {
		return 0.5
	}
	if x < 100 {
		return 1.0
	}
	return 1.5
}
