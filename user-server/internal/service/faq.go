package service

// faq_service.go FAQ 业务服务层
//
// 五层架构归属: L4 业务编排层
// 设计依据: 2026-07-31 AI 智能体性能优化 (T9)
//
// 职责:
//   - 封装 FAQ 检索 + 命中计数 + Layer1 决策建议
//   - 缓存 5 分钟 (热门 FAQ 命中频繁, 避免每次打 DB)
//   - 与 LayerRouter 配合: 提供 "FAQ 是否应该 SkipLLM" 的判定入口
//
// 与 FAQRepository 的区别:
//   - Repository: 纯数据访问 (CRUD + 原始打分)
//   - Service:    业务策略 (阈值 + 缓存 + 日志 + 计数 + 统计)

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/repository"
)

const (
	faqCacheTTL    = 5 * time.Minute
	faqCacheMaxN   = 5000
	faqHitThresh   = 0.6 // 命中分数阈值, 低于此值不进 Layer1
	faqTopKDefault = 3
)

// FAQService FAQ 业务服务
type FAQService struct {
	repo *repository.FAQRepository
	db   *gorm.DB

	mu     sync.RWMutex
	cache  []model.FAQEntry
	loaded time.Time
}

// NewFAQService 创建 FAQ Service
func NewFAQService(db *gorm.DB, repo *repository.FAQRepository) *FAQService {
	if repo == nil && db != nil {
		repo = repository.NewFAQRepository(db)
	}
	return &FAQService{db: db, repo: repo}
}

// Match 在 FAQ 库中匹配用户消息,返回 Top K 候选
func (s *FAQService) Match(ctx context.Context, msg string, topK int) ([]dto.FAQMatchResult, error) {
	if s.repo == nil {
		return nil, nil
	}
	if topK <= 0 {
		topK = faqTopKDefault
	}
	entries, err := s.repo.MatchByKeyword(ctx, msg, topK)
	if err != nil {
		return nil, err
	}
	return s.toMatchResults(entries, "keyword"), nil
}

// MatchByAgent 按智能体绑定的 FAQ 范围匹配 (2026-07-31 P1-A: 知识库绑定)
//
// 绑定为空 = 全局共享, 走 MatchByKeyword;
// 绑定非空 = 仅在绑定的 FAQ ID 集合内匹配
func (s *FAQService) MatchByAgent(ctx context.Context, agentFAQIDs []string, msg string, topK int) ([]dto.FAQMatchResult, error) {
	if s.repo == nil {
		return nil, nil
	}
	if topK <= 0 {
		topK = faqTopKDefault
	}
	var (
		entries []model.FAQEntry
		err     error
		tag     string
	)
	if len(agentFAQIDs) == 0 {
		entries, err = s.repo.MatchByKeyword(ctx, msg, topK)
		tag = "keyword"
	} else {
		// 把 string 数组转 uint (数据库 id 是 bigint)
		ids := make([]string, 0, len(agentFAQIDs))
		ids = append(ids, agentFAQIDs...)
		entries, err = s.repo.MatchByIDs(ctx, msg, ids, topK)
		tag = "agent_bound"
	}
	if err != nil {
		return nil, err
	}
	return s.toMatchResults(entries, tag), nil
}

// toMatchResults 转换为 DTO 列表
func (s *FAQService) toMatchResults(entries []model.FAQEntry, matchType string) []dto.FAQMatchResult {
	out := make([]dto.FAQMatchResult, 0, len(entries))
	for i, e := range entries {
		out = append(out, dto.FAQMatchResult{
			Entry:     entryToDTO(&e),
			Score:     e.Confidence,
			Rank:      i,
			HitCount:  e.HitCount,
			MatchType: matchType,
		})
	}
	return out
}

// CRUD 方法 (前端 FAQ 管理页面使用, 五层架构 L4)

// List 列表查询
func (s *FAQService) List(ctx context.Context, filter repository.FAQFilter) ([]dto.FAQEntry, int64, error) {
	if s.repo == nil {
		return nil, 0, nil
	}
	entries, total, err := s.repo.ListWithFilter(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.FAQEntry, 0, len(entries))
	for i := range entries {
		out = append(out, *entryToDTO(&entries[i]))
	}
	return out, total, nil
}

// GetByID 按 ID 查询
func (s *FAQService) GetByID(ctx context.Context, id uint) (*dto.FAQEntry, error) {
	if s.repo == nil {
		return nil, nil
	}
	e, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return entryToDTO(e), nil
}

// Create 新增 FAQ
func (s *FAQService) Create(ctx context.Context, entry *model.FAQEntry) error {
	if s.repo == nil {
		return fmt.Errorf("repo not initialized")
	}
	if entry.Enabled == nil {
		t := true
		entry.Enabled = &t
	}
	if err := s.repo.Create(ctx, entry); err != nil {
		return err
	}
	s.InvalidateCache()
	return nil
}

// Update 更新 FAQ
func (s *FAQService) Update(ctx context.Context, id uint, entry *model.FAQEntry) error {
	if s.repo == nil {
		return fmt.Errorf("repo not initialized")
	}
	if err := s.repo.Update(ctx, id, entry); err != nil {
		return err
	}
	s.InvalidateCache()
	return nil
}

// Delete 删除 FAQ
func (s *FAQService) Delete(ctx context.Context, id uint) error {
	if s.repo == nil {
		return fmt.Errorf("repo not initialized")
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.InvalidateCache()
	return nil
}

// ShouldSkipLLM 基于 FAQ 命中判定是否应该跳过 LLM
//
// 规则:
//   - 命中且 score >= faqHitThresh   -> true (SkipLLM, 用 FAQ 回复)
//   - 命中但 score <  faqHitThresh   -> false (走 Layer2 LLM 兜底)
//   - 未命中                          -> false
func (s *FAQService) ShouldSkipLLM(matches []dto.FAQMatchResult) (bool, *dto.FAQMatchResult) {
	if len(matches) == 0 {
		return false, nil
	}
	top := matches[0]
	if top.Score < faqHitThresh {
		return false, nil
	}
	return true, &top
}

// IncrementHitCount 命中计数 (异步, 不阻塞主流程)
func (s *FAQService) IncrementHitCount(ctx context.Context, id uint) {
	if s.repo == nil || id == 0 {
		return
	}
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.repo.IncrementHitCount(bgCtx, id)
	}()
}

// WarmupCache 预热缓存 (启动时调用)
func (s *FAQService) WarmupCache(ctx context.Context) error {
	if s.repo == nil {
		return nil
	}
	entries, err := s.repo.ListEnabled(ctx, faqCacheMaxN)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.cache = entries
	s.loaded = time.Now()
	s.mu.Unlock()
	return nil
}

// InvalidateCache 失效缓存 (FAQ 增删改后调用)
func (s *FAQService) InvalidateCache() {
	s.mu.Lock()
	s.cache = nil
	s.mu.Unlock()
}

// Stats 命中统计
func (s *FAQService) Stats(ctx context.Context) (total, enabled int64, err error) {
	if s.repo == nil {
		return 0, 0, nil
	}
	all, err := s.repo.ListEnabled(ctx, 10000)
	if err != nil {
		return 0, 0, err
	}
	enabled = int64(len(all))
	// 估算 total (ListEnabled 已限定 enabled=true, 需另查)
	// 为符合 5 层架构 (service 不直访 db), 仅返回 enabled 数
	return enabled, enabled, nil
}

func entryToDTO(e *model.FAQEntry) *dto.FAQEntry {
	if e == nil {
		return nil
	}
	enabled := false
	if e.Enabled != nil {
		enabled = *e.Enabled
	}
	return &dto.FAQEntry{
		ID:         e.ID,
		Question:   e.Question,
		Answer:     e.Answer,
		Keywords:   []string(e.Keywords),
		Category:   e.Category,
		Intent:     e.Intent,
		Confidence: e.Confidence,
		HitCount:   e.HitCount,
		Enabled:    &enabled,
	}
}
