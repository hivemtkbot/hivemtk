package service

// sop_template_service.go SOP 模板业务服务层
//
// 五层架构归属: L4 业务编排层
// 设计依据: 2026-07-31 AI 智能体性能优化 (T10)
//
// 职责:
//   - 按 (intent, stage) 匹配 SOP 模板
//   - Go text/template 变量替换
//   - 缓存 5 分钟 (模板改动少)
//   - 命中计数

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"text/template"
	"time"

	"gorm.io/gorm"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/repository"
)

const (
	sopCacheTTL  = 5 * time.Minute
	sopCacheMaxN = 2000
	sopTopK      = 5
)

// SOPTemplateService SOP 模板业务服务
type SOPTemplateService struct {
	repo *repository.SOPTemplateRepository
	db   *gorm.DB

	mu     sync.RWMutex
	cache  []model.SOPTemplate
	loaded time.Time
}

// NewSOPTemplateService 创建 SOP Service
func NewSOPTemplateService(db *gorm.DB, repo *repository.SOPTemplateRepository) *SOPTemplateService {
	if repo == nil && db != nil {
		repo = repository.NewSOPTemplateRepository(db)
	}
	return &SOPTemplateService{db: db, repo: repo}
}

// MatchByIntent 按意图匹配
func (s *SOPTemplateService) MatchByIntent(ctx context.Context, intent string) ([]model.SOPTemplate, error) {
	if s.repo == nil || intent == "" {
		return nil, nil
	}
	return s.repo.MatchByIntent(ctx, intent)
}

// MatchByIntentStage 按 (intent, stage) 精确匹配
func (s *SOPTemplateService) MatchByIntentStage(ctx context.Context, intent, stage string) ([]model.SOPTemplate, error) {
	if s.repo == nil {
		return nil, nil
	}
	return s.repo.MatchByIntentStage(ctx, intent, stage)
}

// MatchByAgent 按智能体绑定的 SOP 模板范围匹配 (2026-07-31 P1-A: 知识库绑定)
//
// 绑定为空 = 全局共享, 走 MatchByIntentStage;
// 绑定非空 = 仅在绑定的 SOP template ID 集合内匹配
func (s *SOPTemplateService) MatchByAgent(ctx context.Context, agentSOPIDs []string, intent, stage string) ([]model.SOPTemplate, error) {
	if s.repo == nil {
		return nil, nil
	}
	if len(agentSOPIDs) == 0 {
		return s.repo.MatchByIntentStage(ctx, intent, stage)
	}
	return s.repo.MatchByIDs(ctx, intent, stage, agentSOPIDs)
}

// CRUD 方法 (前端 SOP 模板管理页面使用, 五层架构 L4)

// List 列表查询
func (s *SOPTemplateService) List(ctx context.Context, filter repository.SOPTemplateFilter) ([]dto.SOPTemplate, int64, error) {
	if s.repo == nil {
		return nil, 0, nil
	}
	tpls, total, err := s.repo.ListWithFilter(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.SOPTemplate, 0, len(tpls))
	for i := range tpls {
		out = append(out, *sopToDTO(&tpls[i]))
	}
	return out, total, nil
}

// GetByID 按 ID 查询
func (s *SOPTemplateService) GetByID(ctx context.Context, id uint) (*dto.SOPTemplate, error) {
	if s.repo == nil {
		return nil, nil
	}
	tpl, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return sopToDTO(tpl), nil
}

// Create 新增
func (s *SOPTemplateService) Create(ctx context.Context, tpl *model.SOPTemplate) error {
	if s.repo == nil {
		return fmt.Errorf("repo not initialized")
	}
	if tpl.Enabled == nil {
		t := true
		tpl.Enabled = &t
	}
	if err := s.repo.Create(ctx, tpl); err != nil {
		return err
	}
	s.InvalidateCache()
	return nil
}

// Update 更新
func (s *SOPTemplateService) Update(ctx context.Context, id uint, tpl *model.SOPTemplate) error {
	if s.repo == nil {
		return fmt.Errorf("repo not initialized")
	}
	if err := s.repo.Update(ctx, id, tpl); err != nil {
		return err
	}
	s.InvalidateCache()
	return nil
}

// Delete 删除
func (s *SOPTemplateService) Delete(ctx context.Context, id uint) error {
	if s.repo == nil {
		return fmt.Errorf("repo not initialized")
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.InvalidateCache()
	return nil
}

// sopToDTO 转换 model -> dto
func sopToDTO(t *model.SOPTemplate) *dto.SOPTemplate {
	if t == nil {
		return nil
	}
	enabled := false
	if t.Enabled != nil {
		enabled = *t.Enabled
	}
	return &dto.SOPTemplate{
		ID:         t.ID,
		Name:       t.Name,
		Intent:     t.Intent,
		Stage:      t.Stage,
		Template:   t.Template,
		Vars:       t.Vars,
		Priority:   t.Priority,
		Confidence: t.Confidence,
		HitCount:   t.HitCount,
		Enabled:    &enabled,
	}
}

// Render 渲染模板 (Go text/template)
//
//   - template: 含 {{.var_name}} 占位符的字符串
//   - vars:     map[string]any 变量
func (s *SOPTemplateService) Render(rawTpl string, vars map[string]any) (string, error) {
	if rawTpl == "" {
		return "", nil
	}
	tpl, err := template.New("sop").Option("missingkey=zero").Parse(rawTpl)
	if err != nil {
		return "", fmt.Errorf("sop template parse: %w", err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("sop template execute: %w", err)
	}
	return buf.String(), nil
}

// IncrementHitCount 命中计数 (异步)
func (s *SOPTemplateService) IncrementHitCount(ctx context.Context, id uint) {
	if s.repo == nil || id == 0 {
		return
	}
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.repo.IncrementHitCount(bgCtx, id)
	}()
}

// WarmupCache 预热缓存
func (s *SOPTemplateService) WarmupCache(ctx context.Context) error {
	if s.repo == nil {
		return nil
	}
	tpls, err := s.repo.ListEnabled(ctx, sopCacheMaxN)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.cache = tpls
	s.loaded = time.Now()
	s.mu.Unlock()
	return nil
}

// InvalidateCache 失效缓存
func (s *SOPTemplateService) InvalidateCache() {
	s.mu.Lock()
	s.cache = nil
	s.mu.Unlock()
}

// BuildLayer1Reply 构造 Layer1 SOP 模板回复
func (s *SOPTemplateService) BuildLayer1Reply(tpl *model.SOPTemplate, vars map[string]any) (string, error) {
	if tpl == nil {
		return "", nil
	}
	rendered, err := s.Render(tpl.Template, vars)
	if err != nil {
		return "", err
	}
	return rendered, nil
}

// ShouldSkipLLM SOP 模板是否足以跳过 LLM
func (s *SOPTemplateService) ShouldSkipLLM(tpl *model.SOPTemplate) bool {
	if tpl == nil {
		return false
	}
	return tpl.Confidence >= 0.7
}

// dtoFromTemplate 转 DTO (供 LayerRouter 决策使用)
func sopToLayer(tpl *model.SOPTemplate) *dto.LayerDecision {
	if tpl == nil {
		return nil
	}
	return &dto.LayerDecision{
		Layer:      dto.Layer1,
		SkipLLM:    true,
		Reason:     dto.ReasonSOPHit,
		SOPID:      tpl.ID,
		Intent:     tpl.Intent,
		Confidence: tpl.Confidence,
	}
}
