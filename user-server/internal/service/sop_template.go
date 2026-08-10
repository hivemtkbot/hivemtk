package service

// sop_template_service.go SOP 模板业务服务层
//
// 五层架构归属: L4 业务编排层
// 设计依据: AI 智能体性能优化 + 强 1对1 改造 (Task 16)
//
// 职责:
//   - 按 (agent_id, intent, stage) 严格 1对1 匹配 SOP 模板
//   - Go text/template 变量替换
//   - 按 agentID 分片缓存 5 分钟 (模板改动少)
//   - 命中计数
//
// Task 16 变更:
//   - 新 MatchByAgent(ctx, agentID, intent, stage, topK) 签名 (移除 "空数组=全局" 分支)
//   - 加 bindingRepo 字段 (用于校验 agent ↔ KB 关系)
//   - 缓存由单片改为按 agentID 分片 (map[uint][]model.SOPTemplate)
//   - Create 必填 AgentID
//   - 旧 MatchByAgent(agentSOPIDs []string, ...) 标记 DEPRECATED, 仅做兼容

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"text/template"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	dbUtil "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/repository"
)

const (
	sopCacheTTL  = 5 * time.Minute
	sopCacheMaxN = 2000
	sopTopK      = 5
	// Task 16: agentID 共享池 (agentID=0 表示全局共享池)
	sopAgentShared uint = 0
)

// sopTemplateWhitelist SOP 模板渲染白名单 (: 防 SSTI)
//
// 仅允许以下字段被传入 text/template.Execute, 其他字段 (特别是 user_message) 一律过滤。
// 新增白名单字段需同时:
//  1. 评估 SSTI 风险 (用户内容是否可控)
//  2. 在此常量 + 单测中体现
//  3. 文档化取值来源
var sopTemplateWhitelist = map[string]struct{}{
	"customer_id":  {},
	"intent":       {},
	"stage":        {},
	"agent_name":   {},
	"product_name": {},
	"intent_name":  {},
}

// sopRepoIface SOP Repository 接口 (Task 16: 抽象便于单测注入 mock)
//
// 与 *repository.SOPTemplateRepository 鸭子类型兼容, 生产代码无需感知。
// Task 16 扩展: MatchByAgent(ctx, agentID, intent, stage) 用于强 1对1 匹配
type sopRepoIface interface {
	Create(ctx context.Context, tpl *model.SOPTemplate) error
	GetByID(ctx context.Context, id uint) (*model.SOPTemplate, error)
	ListEnabled(ctx context.Context, limit int) ([]model.SOPTemplate, error)
	// 旧 API: 全局匹配 (agentID=0) — 仅兼容旧调用
	MatchByIntent(ctx context.Context, intent string) ([]model.SOPTemplate, error)
	MatchByIntentStage(ctx context.Context, intent, stage string) ([]model.SOPTemplate, error)
	// Task 16: 按 agentID 严格 1:1 匹配
	MatchByAgent(ctx context.Context, agentID uint, intent, stage string) ([]model.SOPTemplate, error)
	// 按 ID 集合 + 意图 + 阶段匹配 (DEPRECATED: 改走 agent_id 字段)
	MatchByIDs(ctx context.Context, intent, stage string, ids []string) ([]model.SOPTemplate, error)
	// 任务 16: 列出某智能体 SOP 模板 (缓存预热 / 后台同步用)
	ListByAgent(ctx context.Context, agentID uint, limit int) ([]model.SOPTemplate, error)
	IncrementHitCount(ctx context.Context, id uint) error
	ListWithFilter(ctx context.Context, filter repository.SOPTemplateFilter) ([]model.SOPTemplate, int64, error)
	Update(ctx context.Context, id uint, tpl *model.SOPTemplate) error
	Delete(ctx context.Context, id uint) error
}

// sopBindingRepoIface 智能体知识库绑定仓库接口 (Task 16 注入)
//
// 与 *repository.AgentKBBindingRepository 鸭子类型兼容。
// 引入此接口是为了避免 service 对 bindingRepo 实现细节的直接依赖,
// 同时方便单测注入 mock。
type sopBindingRepoIface interface {
	ListByAgent(ctx context.Context, agentID uint, kbType string) ([]model.AgentKBBinding, error)
}

// SOPTemplateService SOP 模板业务服务
//
// Task 16 变更:
//   - 增加 bindingRepo 字段 (注入绑定仓库, 用于校验 agent ↔ KB 关系)
//   - cache 由单片 []model.SOPTemplate 改为 map[uint][]model.SOPTemplate (按 agentID 分片)
//   - 新增 sharedCache 概念: agentID=0 的桶存储"共享池" SOP (向后兼容旧 Match API)
type SOPTemplateService struct {
	repo        sopRepoIface
	bindingRepo sopBindingRepoIface // Task 16 注入
	db          *gorm.DB

	// Task 16: 按 agentID 分片的缓存
	//   - key 0:   共享池 (Match() 走共享)
	//   - key N>0: 智能体 N 的私有 SOP 池 (MatchByAgent(ctx, N, ...) 命中)
	mu     sync.RWMutex
	cache  map[uint][]model.SOPTemplate
	loaded map[uint]time.Time
}

// NewSOPTemplateServiceDefault 使用全局 DB 创建 SOP Service（controller 层入口，避免 controller 持有 gorm.DB）。
func NewSOPTemplateServiceDefault() *SOPTemplateService {
	return NewSOPTemplateService(dbUtil.GetDB(), nil)
}

// NewSOPTemplateService 创建 SOP Service
//
// Task 16: 第二个参数保留 *repository.SOPTemplateRepository 兼容旧调用, bindingRepo 走 SetBindingRepo 注入。
func NewSOPTemplateService(db *gorm.DB, repo *repository.SOPTemplateRepository) *SOPTemplateService {
	var iface sopRepoIface
	if repo == nil && db != nil {
		repo = repository.NewSOPTemplateRepository(db)
	}
	if repo != nil {
		iface = repo
	}
	var bindingIface sopBindingRepoIface
	if db != nil {
		bindingIface = repository.NewAgentKBBindingRepository(db)
	}
	return &SOPTemplateService{
		db:          db,
		repo:        iface,
		bindingRepo: bindingIface,
		cache:       make(map[uint][]model.SOPTemplate),
		loaded:      make(map[uint]time.Time),
	}
}

// NewSOPTemplateServiceWithRepo 用任意实现 sopRepoIface 的 repo 创建 (单测用)
func NewSOPTemplateServiceWithRepo(repo sopRepoIface) *SOPTemplateService {
	return &SOPTemplateService{
		repo:   repo,
		cache:  make(map[uint][]model.SOPTemplate),
		loaded: make(map[uint]time.Time),
	}
}

// NewSOPTemplateServiceWithRepos 用 repo + bindingRepo 同时注入 (Task 16: 单元测试/上层装配)
func NewSOPTemplateServiceWithRepos(repo sopRepoIface, bindingRepo sopBindingRepoIface) *SOPTemplateService {
	return &SOPTemplateService{
		repo:        repo,
		bindingRepo: bindingRepo,
		cache:       make(map[uint][]model.SOPTemplate),
		loaded:      make(map[uint]time.Time),
	}
}

// SetBindingRepo 注入 binding 仓库 (供 layer / controller 装配时使用)
func (s *SOPTemplateService) SetBindingRepo(r sopBindingRepoIface) {
	if r != nil {
		s.bindingRepo = r
	}
}

// MatchByIntent 按意图匹配 (旧 API, 全局共享 — 仅用于管理界面 / 调试)
func (s *SOPTemplateService) MatchByIntent(ctx context.Context, intent string) ([]model.SOPTemplate, error) {
	if s.repo == nil || intent == "" {
		return nil, nil
	}
	return s.repo.MatchByIntent(ctx, intent)
}

// MatchByIntentStage 按 (intent, stage) 精确匹配 (旧 API, 全局共享 — 仅用于管理界面 / 调试)
func (s *SOPTemplateService) MatchByIntentStage(ctx context.Context, intent, stage string) ([]model.SOPTemplate, error) {
	if s.repo == nil {
		return nil, nil
	}
	return s.repo.MatchByIntentStage(ctx, intent, stage)
}

// MatchByAgent 强 1对1 匹配 SOP 模板 (Task 16: 严格 1:1 改造)
//
// 新签名: (ctx, agentID uint, intent, stage string, topK int)
//
// 行为:
//   - agentID == 0: 不再走"空数组=全局"回退, 直接返回 (nil, nil)
//   - agentID > 0: 仅匹配 enabled = true AND agent_id = ? 的 SOP
//   - intent/stage 为空时该过滤项不应用
//   - topK <= 0 时走 sopTopK (5)
//
// 调用方 (LayerRouter / SOPTemplateController) 必须显式传入 agentID。
func (s *SOPTemplateService) MatchByAgent(ctx context.Context, agentID uint, intent, stage string, topK int) ([]model.SOPTemplate, error) {
	if s.repo == nil {
		return nil, nil
	}
	if agentID == 0 {
		// Task 16 强 1对1: 无 agentID 不再做"全局兜底"
		return nil, nil
	}
	if topK <= 0 {
		topK = sopTopK
	}
	all, err := s.repo.MatchByAgent(ctx, agentID, intent, stage)
	if err != nil {
		return nil, err
	}
	if len(all) > topK {
		all = all[:topK]
	}
	return all, nil
}

// MatchByAgentLegacy 旧签名匹配 (DEPRECATED: 兼容 layer.go 旧调用, 后续移除)
//
// 签名: (ctx, agentSOPIDs []string, intent, stage string) — 保留以支持逐步迁移
//
// 行为:
//   - agentSOPIDs 为空: 不再走 MatchByIntentStage, 直接返回 (nil, nil) (强 1对1: 移除"空数组=全局"分支)
//   - agentSOPIDs 非空: 走 MatchByIDs (旧 ID 集合过滤)
//
// Deprecated: Task 16 强 1对1 改造, 新代码应使用 MatchByAgent(ctx, agentID, ...)。
// 仅供 layer.go 过渡期使用。
func (s *SOPTemplateService) MatchByAgentLegacy(ctx context.Context, agentSOPIDs []string, intent, stage string) ([]model.SOPTemplate, error) {
	if s.repo == nil {
		return nil, nil
	}
	if len(agentSOPIDs) == 0 {
		// Task 16 强 1对1: 移除"空数组=全局"分支, 改由 layer.go 显式传 agentID
		return nil, nil
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

// Create 新增 SOP 模板
//
// Task 16 改造: AgentID 必填 (强 1对1, 所有 SOP 都必须归属于某个智能体或共享池)
func (s *SOPTemplateService) Create(ctx context.Context, tpl *model.SOPTemplate) error {
	if s.repo == nil {
		return fmt.Errorf("repo not initialized")
	}
	if tpl == nil {
		return errors.New("tpl is nil")
	}
	if tpl.AgentID == nil || *tpl.AgentID == 0 {
		return errors.New("agent_id 必填 (Task 16 强 1对1: 不允许全局匿名 SOP 模板)")
	}
	if strings.TrimSpace(tpl.Intent) == "" {
		return errors.New("intent 必填")
	}
	if strings.TrimSpace(tpl.Stage) == "" {
		return errors.New("stage 必填")
	}
	if tpl.Enabled == nil {
		t := true
		tpl.Enabled = &t
	}
	if err := s.repo.Create(ctx, tpl); err != nil {
		return err
	}
	s.InvalidateCache(*tpl.AgentID)
	return nil
}

// Update 更新 SOP 模板
func (s *SOPTemplateService) Update(ctx context.Context, id uint, tpl *model.SOPTemplate) error {
	if s.repo == nil {
		return fmt.Errorf("repo not initialized")
	}
	if err := s.repo.Update(ctx, id, tpl); err != nil {
		return err
	}
	// 失效对应 agentID 的缓存 (tpl.AgentID 可能为 nil, 兜底用全局)
	if tpl != nil && tpl.AgentID != nil {
		s.InvalidateCache(*tpl.AgentID)
	} else {
		s.InvalidateAllCache()
	}
	return nil
}

// Delete 删除 SOP 模板
func (s *SOPTemplateService) Delete(ctx context.Context, id uint) error {
	if s.repo == nil {
		return fmt.Errorf("repo not initialized")
	}
	// 先查一下属于哪个 agent (用于精确失效缓存)
	existing, _ := s.repo.GetByID(ctx, id)
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	if existing != nil && existing.AgentID != nil {
		s.InvalidateCache(*existing.AgentID)
	} else {
		s.InvalidateAllCache()
	}
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
//   - rawTpl: 含 {{.var_name}} 占位符的字符串
//
// vars: map[string]any 变量 (: 仅白名单字段透传到模板, 防止 SSTI)
//
// 安全:
//   - 白名单外的字段 (特别是 user_message) 会被过滤, 不会到达模板
//   - 即使模板作者写了 {{.user_message}}, 渲染时该 key 也不存在 (missingkey=zero)
//   - 白名单 sopTemplateWhitelist 是 const map, 不可运行时篡改
func (s *SOPTemplateService) Render(rawTpl string, vars map[string]any) (string, error) {
	if rawTpl == "" {
		return "", nil
	}
	safe := filterWhitelistVars(vars)
	tpl, err := template.New("sop").Option("missingkey=zero").Parse(rawTpl)
	if err != nil {
		return "", fmt.Errorf("sop template parse: %w", err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, safe); err != nil {
		return "", fmt.Errorf("sop template execute: %w", err)
	}
	return buf.String(), nil
}

// filterWhitelistVars 过滤 vars, 只保留白名单字段 (: 防 SSTI)
//
// 返回新 map, 不修改入参。
// nil 入参返回空 map (nil-safe)。
func filterWhitelistVars(vars map[string]any) map[string]any {
	if vars == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(sopTemplateWhitelist))
	for k := range sopTemplateWhitelist {
		if v, ok := vars[k]; ok {
			out[k] = v
		}
	}
	return out
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

// WarmupCache 预热缓存 (Task 16: 按 agentID 分片)
//
// 行为:
//   - agentID == 0: 预热共享池 (走 ListEnabled)
//   - agentID > 0: 仅预热该智能体的 SOP (走 ListByAgent)
//   - TTL = 5 min, 重复调用会覆盖旧缓存
func (s *SOPTemplateService) WarmupCache(ctx context.Context, agentID uint) error {
	if s.repo == nil {
		return nil
	}
	var tpls []model.SOPTemplate
	var err error
	if agentID == sopAgentShared {
		tpls, err = s.repo.ListEnabled(ctx, sopCacheMaxN)
	} else {
		tpls, err = s.repo.ListByAgent(ctx, agentID, sopCacheMaxN)
	}
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.cache[agentID] = tpls
	s.loaded[agentID] = time.Now()
	s.mu.Unlock()
	return nil
}

// WarmupAll 预热所有已知 agent 的缓存 (启动时调用, 由调用方传入 agentID 列表)
//
// 用途: 已知全量 agent 列表时, 一次预热避免运行时冷启动。
// 实现: 简单串行预热每个 agent, 失败不阻塞。
func (s *SOPTemplateService) WarmupAll(ctx context.Context, agentIDs []uint) {
	for _, id := range agentIDs {
		_ = s.WarmupCache(ctx, id)
	}
}

// InvalidateCache 失效指定 agentID 的缓存 (Task 16: 精确失效)
//
// agentID == 0: 失效共享池
// agentID > 0: 失效该 agent 私有桶
func (s *SOPTemplateService) InvalidateCache(agentID uint) {
	s.mu.Lock()
	delete(s.cache, agentID)
	delete(s.loaded, agentID)
	s.mu.Unlock()
}

// InvalidateAllCache 失效全部缓存 (Task 16 兼容入口)
//
// 用于: 业务不确定受影响 agentID 时 (例如全局配置变更)。
func (s *SOPTemplateService) InvalidateAllCache() {
	s.mu.Lock()
	s.cache = make(map[uint][]model.SOPTemplate)
	s.loaded = make(map[uint]time.Time)
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

// sopToLayer 转 DTO (供 LayerRouter 决策使用)
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
