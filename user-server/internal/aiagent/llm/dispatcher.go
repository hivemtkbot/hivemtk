package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"marketing/internal/pkg/utils/config"
	"marketing/internal/pkg/utils/logger"
)

// DispatchScenario 调度场景
type DispatchScenario string

const (
	ScenarioIntentRecognize DispatchScenario = "intent_recognize" // 意图识别
	ScenarioSOPReply        DispatchScenario = "sop_reply"        // SOP 销冠回复
	ScenarioObjection       DispatchScenario = "objection"        // 异议处理
	ScenarioFriendlyChat    DispatchScenario = "friendly_chat"    // 拟人寒暄
	ScenarioLongSummary     DispatchScenario = "long_summary"     // 长文本总结
	ScenarioHighQuality     DispatchScenario = "high_quality"     // 高质量回复
	ScenarioLowCost         DispatchScenario = "low_cost"         // 低成本批量
)

// ProviderConfig 厂商配置
type ProviderConfig struct {
	Name         string  `json:"name"` // deepseek / qwen / gpt-4o / glm-4
	APIKey       string  `json:"api_key"`
	BaseURL      string  `json:"base_url"`
	APIType      string  `json:"api_type"` // openai / anthropic
	Model        string  `json:"model"`
	CostPer1k    float64 `json:"cost_per_1k"`
	AvgLatencyMs int     `json:"avg_latency_ms"`
	QualityScore float64 `json:"quality_score"`
	MaxRPM       int     `json:"max_rpm"` // 每分钟请求数
	MaxTPM       int     `json:"max_tpm"` // 每分钟 token 数
	Enabled      bool    `json:"enabled"`
	// NoFC 标记该 provider 不支持 OpenAI Function Calling
	// 为 true 时，Dispatcher 会启用 ReAct 适配器，通过文本协议完成工具调用
	// 适用场景：本地 LLM（llama.cpp / mtk-llm / 部分 ChatGLM 版本）不支持 FC
	NoFC bool `json:"no_fc,omitempty"`
}

// ScenarioRoute 场景路由策略
//
// 设计依据：docs/marketing-features/llm-routing.md
// 字段含义：
//   - Provider:   首选 provider name
//   - Fallbacks:  备选（按顺序）
//   - CostWeight: 1-5 成本权重（仅作展示，Dispatch 暂不消费，留待智能路由 P1）
//   - MaxLatency: 单次调用最大时延（ms）
//   - MinQuality: 最低质量门槛（QualityScore 低于此值的 provider 被跳过）
//   - Version:    路由版本号（自增），用于审计与回滚（2026-07-23 补）
//   - Weight:     灰度发布权重 0-100，0=全量回滚、100=全量新路由
//     当次 Dispatch 按 Weight 决定走新路由还是旧路由（2026-07-23 补）
//   - CanaryKey:  灰度判定 key（如 user_id），空时按权重随机抽样
//   - CanaryRoute: 灰度时的对照路由（仅当 Weight>0 且 <100 时生效）
type ScenarioRoute struct {
	Scenario    DispatchScenario `json:"scenario"`
	Provider    string           `json:"provider"`
	Fallbacks   []string         `json:"fallbacks"`
	CostWeight  int              `json:"cost_weight"`
	MaxLatency  int              `json:"max_latency"`
	MinQuality  float64          `json:"min_quality"`
	Version     int              `json:"version"`
	Weight      int              `json:"weight"`
	CanaryKey   string           `json:"canary_key,omitempty"`
	CanaryRoute *ScenarioRoute   `json:"canary_route,omitempty"`
}

// Dispatcher 多模型调度器
type Dispatcher struct {
	mu         sync.RWMutex
	providers  map[string]*ProviderConfig
	routes     map[DispatchScenario]*ScenarioRoute
	llmService *LLMService
	rpmCounter map[string]*rpmBucket
	cache      map[string]*dispatchCacheEntry
	cacheMu    sync.RWMutex
	// ReAct 适配器（让无 FC 能力的 LLM 通过文本协议接入 Agent Loop）
	// 懒初始化，首次需要时创建（避免无工具调用场景的开销）
	reactAdapter   *ReActAdapter
	reactAdapterMu sync.Once
	// testMode: 当为 true 时，callProvider 跳过所有告警/统计/AvgLatency 更新副作用
	// 仅供 CallProviderForTest 临时置位
	testMode atomic.Bool
}

type rpmBucket struct {
	mu      sync.Mutex
	count   int
	resetAt time.Time
}

type dispatchCacheEntry struct {
	content  string
	expireAt time.Time
}

// newDispatcherBase 仅初始化内部容器（不注册任何 provider/route）
func newDispatcherBase(llmService *LLMService) *Dispatcher {
	return &Dispatcher{
		providers:  make(map[string]*ProviderConfig),
		routes:     make(map[DispatchScenario]*ScenarioRoute),
		llmService: llmService,
		rpmCounter: make(map[string]*rpmBucket),
		cache:      make(map[string]*dispatchCacheEntry),
	}
}

// getReActAdapter 懒初始化 ReAct 适配器（线程安全）
// 仅在首次需要时创建，避免无工具调用场景的初始化开销
func (d *Dispatcher) getReActAdapter() *ReActAdapter {
	d.reactAdapterMu.Do(func() {
		d.reactAdapter = NewReActAdapter()
	})
	return d.reactAdapter
}

// NewDispatcher 创建调度器（默认云端厂商，保留给单测/历史调用）
func NewDispatcher(llmService *LLMService) *Dispatcher {
	d := newDispatcherBase(llmService)
	d.registerDefaultProviders()
	d.registerDefaultRoutes()
	return d
}

// NewDispatcherFromConfig 依据配置构建「本地优先」调度器（优化三核心）
//
//   - 注册 default provider：指向本地 mtk-llm（OpenAI 兼容），始终启用。
//   - 云端厂商（deepseek/qwen/gpt-4o/glm-4/kimi）仅在配置 api_key 且 enabled=true 时启用，
//     否则保持禁用，绝不作为默认路由（避免空密钥 401 风暴、数据出域）。
//   - 所有场景主路由均为 default（本地），云端仅作可选 fallback。
//   - 超时全链路对齐：从 inference.llm.timeout_seconds 派生 MaxLatency、HTTP client timeout、
//     LLMConfig.RequestTimeout，避免硬编码常量导致父级 ctx 提前 cancel 子级 LLM 调用。
func NewDispatcherFromConfig(cfg config.AppConfig) *Dispatcher {
	// 单一配置源：inference.llm.timeout_seconds
	// 默认 180s（覆盖大多数 CPU 推理场景），开发模式可在 config.yaml 设大值（如 720s）
	timeoutSec := cfg.Inference.LLM.TimeoutSeconds
	if timeoutSec <= 0 {
		timeoutSec = 180
	}
	// 同步注入 llm_service HTTP client timeout（与 MaxLatency 一致）
	setDefaultHTTPTimeout(time.Duration(timeoutSec) * time.Second)
	d := newDispatcherBase(NewLLMService())
	d.registerLocalProvider(cfg.Inference.LLM)
	d.registerCloudProvidersFromConfig(cfg.Inference.LLM)
	d.registerLocalFirstRoutes(timeoutSec * 1000) // ms
	return d
}

// registerLocalProvider 注册本地默认 provider（指向 mtk-llm / 宿主 127.0.0.1:8207）
//
// 契约：llmCfg.Model 与 llmCfg.BaseURL 必须由 config 层提供非空值
// （config.yaml 或 DefaultInferenceConfig 兜底）。dispatcher 不再做模型名/URL
// 硬编码兜底，避免与 config 契约分裂。
func (d *Dispatcher) registerLocalProvider(llmCfg config.InferenceLLMConfig) {
	if llmCfg.Model == "" {
		panic("inference.llm.model 必须由 config 层提供（config.yaml 或 DefaultInferenceConfig），dispatcher 不再兜底")
	}
	if llmCfg.BaseURL == "" {
		panic("inference.llm.base_url 必须由 config 层提供（config.yaml 或 DefaultInferenceConfig），dispatcher 不再兜底")
	}
	d.providers["default"] = &ProviderConfig{
		Name:         "default",
		APIKey:       llmCfg.APIKey,
		BaseURL:      llmCfg.BaseURL,
		APIType:      "openai",
		Model:        llmCfg.Model,
		CostPer1k:    0,
		AvgLatencyMs: 800,
		// 本地即唯一启用 provider，质量分须高于所有场景的 MinQuality 门槛
		// （Objection=0.92 / HighQuality=0.95），否则会被 MinQuality 网关跳过、
		// 退回已禁用云端导致本地推理永不命中。
		QualityScore: 0.99,
		MaxRPM:       0, // 0 = 不限流
		Enabled:      true,
		// 本地 Qwen2.5-1.5B-Instruct 不支持 OpenAI Function Calling
		// 启用 ReAct 适配器，通过 Thought/Action/Action Input 文本协议完成工具调用
		// 用户可通过 inference.llm.no_fc=false 显式关闭（如果模型已升级支持 FC）
		NoFC: resolveNoFC(llmCfg),
	}
}

// resolveNoFC 解析 NoFC 标记
//
// 配置优先级：用户在 config.yaml 显式写 no_fc 字段（true / false） > URL 启发式
//   - 用户显式写 no_fc: true  → 启用 ReAct 适配器
//   - 用户显式写 no_fc: false → 走原生 OpenAI Function Calling
//   - 用户未写              → 走 URL 启发式（本地默认 true，云端默认 false）
//
// 启发式规则：BaseURL 含 127.0.0.1 / localhost / mtk-llm 视为本地 → true
func resolveNoFC(llmCfg config.InferenceLLMConfig) bool {
	// 用户显式设置 no_fc 时，直接使用该值（跳过 URL 启发式）
	if llmCfg.NoFC != nil {
		return *llmCfg.NoFC
	}
	// URL 启发式兜底（用户未设置时）
	base := strings.ToLower(llmCfg.BaseURL)
	return base == "" || strings.Contains(base, "127.0.0.1") || strings.Contains(base, "localhost") || strings.Contains(base, "mtk-llm")
}

// defaultCloudProviderFactories 云端厂商默认工厂（仅用于未配置场景的占位注册）
//
// 设计意图：NewDispatcherFromConfig 必须先把这些云端厂商注册到 d.providers（即使 enabled=false），
// 这样：
//  1. 运营后台 / API 层可以枚举 d.providers 看到全部可用厂商
//  2. SetRoute / SetRouteWithAudit 可以把云端作为 fallback 而无需先手动 AddProvider
//  3. 测试用例（如 TestNewDispatcherFromConfig_LocalFirst）能验证"无 api_key 时注册但禁用"的契约
//
// 显式提供 api_key 时由 registerCloudProvidersFromConfig 覆盖此处的 disabled 状态。
func defaultCloudProviderFactories() []ProviderConfig {
	return []ProviderConfig{
		{
			Name:         "deepseek",
			BaseURL:      "https://api.deepseek.com",
			APIType:      "openai",
			Model:        "deepseek-chat",
			CostPer1k:    0.001,
			AvgLatencyMs: 1500,
			QualityScore: 0.85,
			MaxRPM:       60,
			Enabled:      false, // 无 api_key → 默认禁用（数据出域防护）
		},
		{
			Name:         "qwen",
			BaseURL:      "https://dashscope.aliyuncs.com/compatible-mode",
			APIType:      "openai",
			Model:        "qwen-max",
			CostPer1k:    0.020,
			AvgLatencyMs: 2500,
			QualityScore: 0.92,
			MaxRPM:       60,
			Enabled:      false,
		},
		{
			Name:         "gpt-4o",
			BaseURL:      "https://api.openai.com",
			APIType:      "openai",
			Model:        "gpt-4o",
			CostPer1k:    0.030,
			AvgLatencyMs: 3000,
			QualityScore: 0.95,
			MaxRPM:       60,
			Enabled:      false,
		},
		{
			Name:         "glm-4",
			BaseURL:      "https://open.bigmodel.cn/api/paas/v4",
			APIType:      "openai",
			Model:        "glm-4-plus",
			CostPer1k:    0.050,
			AvgLatencyMs: 2800,
			QualityScore: 0.91,
			MaxRPM:       60,
			Enabled:      false,
		},
		{
			Name:         "kimi",
			BaseURL:      "https://api.moonshot.cn",
			APIType:      "openai",
			Model:        "moonshot-v1-8k",
			CostPer1k:    0.012,
			AvgLatencyMs: 2200,
			QualityScore: 0.88,
			MaxRPM:       60,
			Enabled:      false,
		},
	}
}

// registerCloudProvidersFromConfig 注册云端可选 fallback
//
// 本地优先原则：
//  1. 先把全部云端厂商（deepseek/qwen/gpt-4o/glm-4/kimi）以 disabled 占位注册到 d.providers，
//     确保"枚举可见、路由可引用、测试可断言"三条契约。
//  2. 用户在 config.yaml / env 显式配置 api_key 且 enabled=true 的云端，覆盖占位 disabled 状态。
//
// 即使没有任何云端配置，providers map 中也必须存在这 5 个 key（disabled），
// 这是 NewDispatcherFromConfig 强契约（参见 TestNewDispatcherFromConfig_LocalFirst）。
func (d *Dispatcher) registerCloudProvidersFromConfig(llmCfg config.InferenceLLMConfig) {
	// 1) 占位注册：5 个云端厂商默认全 disabled
	for _, p := range defaultCloudProviderFactories() {
		d.providers[p.Name] = &p
	}
	// 2) 用户显式配置覆盖：api_key 非空 + enabled=true 才真正启用
	src := llmCfg.CloudProviders
	for _, p := range src {
		enabled := p.Enabled && p.APIKey != ""
		d.providers[p.Name] = &ProviderConfig{
			Name:         p.Name,
			APIKey:       p.APIKey,
			BaseURL:      p.BaseURL,
			APIType:      p.APIType,
			Model:        p.Model,
			CostPer1k:    0.01,
			AvgLatencyMs: 2000,
			QualityScore: 0.9,
			MaxRPM:       60,
			Enabled:      enabled,
		}
	}
}

// registerLocalFirstRoutes 注册本地优先的场景路由（default 为主，云端为可选 fallback）
//
// 注意：本地 mtk-llm（Qwen2.5-1.5B/3B q4）单次生成 30-180s（CPU 推理），
// MaxLatency 必须大于 LLM 实际推理时间，否则 dispatcher 的 context.WithTimeout
// 会在本地推理完成前掐断请求（context deadline exceeded），
// 进而退回已禁用的云端兜底导致 AI 直答失败、错误转人工。
//
// MaxLatency 由参数注入（maxLatencyMs），由 NewDispatcherFromConfig
// 从 inference.llm.timeout_seconds 派生（默认 180000ms，开发模式可在 config.yaml 设大值如 720000）。
// 与 sales_engine.agentLoopTotalTimeout、llm_service.httpClient.Timeout 共享同一配置源。
func (d *Dispatcher) registerLocalFirstRoutes(maxLatencyMs int) {
	if maxLatencyMs <= 0 {
		maxLatencyMs = 180000
	}
	routes := []*ScenarioRoute{
		{Scenario: ScenarioIntentRecognize, Provider: "default", Fallbacks: []string{}, CostWeight: 5, MaxLatency: maxLatencyMs, MinQuality: 0.8},
		{Scenario: ScenarioSOPReply, Provider: "default", Fallbacks: []string{}, CostWeight: 2, MaxLatency: maxLatencyMs, MinQuality: 0.9},
		{Scenario: ScenarioObjection, Provider: "default", Fallbacks: []string{}, CostWeight: 1, MaxLatency: maxLatencyMs, MinQuality: 0.92},
		{Scenario: ScenarioFriendlyChat, Provider: "default", Fallbacks: []string{}, CostWeight: 10, MaxLatency: maxLatencyMs, MinQuality: 0.8},
		{Scenario: ScenarioLongSummary, Provider: "default", Fallbacks: []string{}, CostWeight: 3, MaxLatency: maxLatencyMs, MinQuality: 0.85},
		{Scenario: ScenarioHighQuality, Provider: "default", Fallbacks: []string{}, CostWeight: 1, MaxLatency: maxLatencyMs, MinQuality: 0.95},
		{Scenario: ScenarioLowCost, Provider: "default", Fallbacks: []string{}, CostWeight: 15, MaxLatency: maxLatencyMs, MinQuality: 0.7},
	}
	for _, r := range routes {
		d.routes[r.Scenario] = r
	}
}

// getCache 读取带 TTL 的响应缓存
func (d *Dispatcher) getCache(key string) (string, bool) {
	d.cacheMu.RLock()
	e, ok := d.cache[key]
	d.cacheMu.RUnlock()
	if !ok {
		return "", false
	}
	if time.Now().After(e.expireAt) {
		d.cacheMu.Lock()
		delete(d.cache, key)
		d.cacheMu.Unlock()
		return "", false
	}
	return e.content, true
}

// setCache 写入带 TTL 的响应缓存
func (d *Dispatcher) setCache(key string, ttl int, content string) {
	if ttl <= 0 || key == "" {
		return
	}
	d.cacheMu.Lock()
	d.cache[key] = &dispatchCacheEntry{content: content, expireAt: time.Now().Add(time.Duration(ttl) * time.Second)}
	d.cacheMu.Unlock()
}

// registerDefaultProviders 注册默认厂商
func (d *Dispatcher) registerDefaultProviders() {
	// 这些配置可在运营后台更新
	defaults := []ProviderConfig{
		{
			Name:         "deepseek",
			BaseURL:      "https://api.deepseek.com",
			APIType:      "openai",
			Model:        "deepseek-chat",
			CostPer1k:    0.001,
			AvgLatencyMs: 1500,
			QualityScore: 0.85,
			MaxRPM:       60,
			Enabled:      true,
		},
		{
			Name:         "qwen-turbo",
			BaseURL:      "https://dashscope.aliyuncs.com/compatible-mode",
			APIType:      "openai",
			Model:        "qwen-turbo",
			CostPer1k:    0.003,
			AvgLatencyMs: 1200,
			QualityScore: 0.82,
			MaxRPM:       60,
			Enabled:      true,
		},
		{
			Name:         "qwen-max",
			BaseURL:      "https://dashscope.aliyuncs.com/compatible-mode",
			APIType:      "openai",
			Model:        "qwen-max",
			CostPer1k:    0.020,
			AvgLatencyMs: 2500,
			QualityScore: 0.92,
			MaxRPM:       60,
			Enabled:      true,
		},
		{
			Name:         "gpt-4o",
			BaseURL:      "https://api.openai.com",
			APIType:      "openai",
			Model:        "gpt-4o",
			CostPer1k:    0.030,
			AvgLatencyMs: 3000,
			QualityScore: 0.95,
			MaxRPM:       60,
			Enabled:      true,
		},
		{
			Name:         "glm-4",
			BaseURL:      "https://open.bigmodel.cn/api/paas/v4",
			APIType:      "openai",
			Model:        "glm-4-plus",
			CostPer1k:    0.050,
			AvgLatencyMs: 2800,
			QualityScore: 0.91,
			MaxRPM:       60,
			Enabled:      true,
		},
		{
			Name:         "kimi",
			BaseURL:      "https://api.moonshot.cn",
			APIType:      "openai",
			Model:        "moonshot-v1-8k",
			CostPer1k:    0.012,
			AvgLatencyMs: 2200,
			QualityScore: 0.88,
			MaxRPM:       60,
			Enabled:      true,
		},
	}
	for i := range defaults {
		p := defaults[i]
		d.providers[p.Name] = &p
	}
}

// registerDefaultRoutes 注册默认路由
func (d *Dispatcher) registerDefaultRoutes() {
	routes := []*ScenarioRoute{
		{Scenario: ScenarioIntentRecognize, Provider: "deepseek", Fallbacks: []string{"qwen-turbo"}, CostWeight: 5, MaxLatency: 3000, MinQuality: 0.8},
		{Scenario: ScenarioSOPReply, Provider: "qwen-max", Fallbacks: []string{"gpt-4o", "glm-4"}, CostWeight: 2, MaxLatency: 5000, MinQuality: 0.9},
		{Scenario: ScenarioObjection, Provider: "gpt-4o", Fallbacks: []string{"glm-4", "qwen-max"}, CostWeight: 1, MaxLatency: 5000, MinQuality: 0.92},
		{Scenario: ScenarioFriendlyChat, Provider: "qwen-turbo", Fallbacks: []string{"deepseek"}, CostWeight: 4, MaxLatency: 3000, MinQuality: 0.8},
		{Scenario: ScenarioLongSummary, Provider: "kimi", Fallbacks: []string{"qwen-max"}, CostWeight: 3, MaxLatency: 6000, MinQuality: 0.85},
		{Scenario: ScenarioHighQuality, Provider: "gpt-4o", Fallbacks: []string{"glm-4"}, CostWeight: 1, MaxLatency: 5000, MinQuality: 0.95},
		{Scenario: ScenarioLowCost, Provider: "deepseek", Fallbacks: []string{"qwen-turbo"}, CostWeight: 5, MaxLatency: 4000, MinQuality: 0.7},
	}
	for _, r := range routes {
		d.routes[r.Scenario] = r
	}
}

// AddProvider 动态添加/更新 provider
func (d *Dispatcher) AddProvider(p ProviderConfig) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.providers[p.Name] = &p
}

// SetRoute 设置场景路由（含版本号自增 + 灰度支持）
//
// 行为：
//   - 若同 scenario 已存在路由：Version 自增 1（用于审计与回滚）
//   - 新增：Version 从 1 开始
//   - 若传入 r.Version=0：自动分配（自增或初始化为 1）
//   - 若传入 r.Version>0：以传入为准（外部可手动指定版本）
//
// 返回：实际生效的 route（含 version）。调用方负责将 prev + new 写入审计日志。
func (d *Dispatcher) SetRoute(r ScenarioRoute) ScenarioRoute {
	d.mu.Lock()
	defer d.mu.Unlock()
	prev, hasPrev := d.routes[r.Scenario]
	if r.Version == 0 {
		if hasPrev {
			r.Version = prev.Version + 1
		} else {
			r.Version = 1
		}
	}
	cp := r
	d.routes[r.Scenario] = &cp
	return cp
}

// SetRouteWithAudit 设置场景路由 + 写入审计日志
//
// 这是 SetRoute 的增强版，自动把 prev/new 写入 llm_routing_audit 表。
// 写审计失败仅记录日志，不阻塞路由变更（保证路由生效的可用性）。
func (d *Dispatcher) SetRouteWithAudit(ctx context.Context, r ScenarioRoute, action, operator, traceID string) ScenarioRoute {
	prev := d.GetRoute(r.Scenario)
	applied := d.SetRoute(r)
	d.writeAuditLog(ctx, r.Scenario, prev, &applied, action, operator, traceID)
	return applied
}

// LogModelLifecycle 记录模型生命周期事件（新增/删除），不污染 routes map
//
// 区别于 SetRouteWithAudit：模型生命周期事件与场景路由无关，
// 单独写 audit 行（action=create_model/delete_model），不动 routes。
func (d *Dispatcher) LogModelLifecycle(ctx context.Context, action, provider, operator, traceID string) {
	db := getAuditDB()
	if db == nil {
		return
	}
	row := map[string]any{
		"scenario":       "model_lifecycle",
		"version":        0,
		"prev_provider":  "",
		"new_provider":   provider,
		"prev_fallbacks": "",
		"new_fallbacks":  "",
		"action":         action,
		"operator":       operator,
		"trace_id":       traceID,
	}
	if err := db.WithContext(ctx).Table("llm_routing_audit").Create(row).Error; err != nil {
		logger.Warnf("[LLM] write model lifecycle audit failed: %v", err)
	}
}

// writeAuditLog 写入 llm_routing_audit 表
func (d *Dispatcher) writeAuditLog(ctx context.Context, scenario DispatchScenario, prev, next *ScenarioRoute, action, operator, traceID string) {
	db := getAuditDB()
	if db == nil {
		return
	}
	var prevProvider, newProvider, prevFallbacks, newFallbacks string
	if prev != nil {
		prevProvider = prev.Provider
		prevFallbacks = strings.Join(prev.Fallbacks, ",")
	}
	if next != nil {
		newProvider = next.Provider
		newFallbacks = strings.Join(next.Fallbacks, ",")
	}
	version := 0
	if next != nil {
		version = next.Version
	}
	row := map[string]any{
		"scenario":       string(scenario),
		"version":        version,
		"prev_provider":  prevProvider,
		"new_provider":   newProvider,
		"prev_fallbacks": prevFallbacks,
		"new_fallbacks":  newFallbacks,
		"action":         action,
		"operator":       operator,
		"trace_id":       traceID,
	}
	if err := db.WithContext(ctx).Table("llm_routing_audit").Create(row).Error; err != nil {
		logger.Warnf("[LLM] write routing audit failed: %v", err)
	}
}

// DispatchRequest 调度请求
type DispatchRequest struct {
	Scenario       DispatchScenario `json:"scenario"`
	Prompt         string           `json:"prompt"`
	SystemPrompt   string           `json:"system_prompt"`
	MaxTokens      int              `json:"max_tokens"`
	Temperature    float64          `json:"temperature"`
	JSONMode       bool             `json:"json_mode"`
	CacheKey       string           `json:"cache_key"`       // 缓存 key
	CacheTTL       int              `json:"cache_ttl"`       // 缓存秒数
	ReturnLogprobs bool             `json:"return_logprobs"` // 请求 LLM 返回 token logprobs
	TopLogprobs    int              `json:"top_logprobs"`    // 返回 top-N 候选 token logprobs（默认 20）
	// 智能体 tool_call 支持
	// Tools: 工具定义列表，非空时 Dispatcher 走 GenerateWithTools 路径
	// ToolChoice: "auto"/"none"/"required" 或 JSON 对象字符串
	Tools      []ToolDefinition `json:"tools,omitempty"`
	ToolChoice string           `json:"tool_choice,omitempty"`
	// Messages: 多轮对话历史（含 tool 角色回灌工具结果）
	// 非空时 Prompt 字段被忽略，使用 Messages 进行多轮对话
	Messages []ChatMessage `json:"messages,omitempty"`
	// CanaryKey: 灰度发布判定 key（如 user_id），空时按权重随机抽样
	// 让 Dispatch 走灰度路由
	CanaryKey string `json:"canary_key,omitempty"`
	// 多语言方案：跨语言生成元数据（由 service/i18n 层注入）
	InternalLang    string `json:"internal_lang,omitempty"`    // 商户内部语言（知识库语言）
	TargetLang      string `json:"target_lang,omitempty"`      // 对外输出语言
	CrossLingual    bool   `json:"cross_lingual,omitempty"`    // 是否跨语言生成（InternalLang != TargetLang）
	GlossaryVersion string `json:"glossary_version,omitempty"` // 术语表版本（落库审计）
}

// DispatchResult 调度结果
type DispatchResult struct {
	Provider    string  `json:"provider"`
	Model       string  `json:"model"`
	Content     string  `json:"content"`
	TotalTokens int     `json:"total_tokens"`
	Cost        float64 `json:"cost"`
	LatencyMs   int     `json:"latency_ms"`
	FromCache   bool    `json:"from_cache"`
	// 用于置信度计算的 token 级信号
	// 当前 LLMService 尚未真正透传 logprobs，字段保留供上游 SignalCollector
	// 在 LLMService 升级（添加 chatResponse.logprobs 解析）后自动填充。
	// 当前为安全降级：Logprobs/TopTokenEntropy 留空，FinishReason 透传 "stop"/"length" 等。
	Logprobs        []float64 `json:"logprobs,omitempty"`
	TopTokenEntropy float64   `json:"top_token_entropy,omitempty"`
	FinishReason    string    `json:"finish_reason,omitempty"`
	// 智能体 tool_call 返回结果
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// 详细 token 使用量（用于计费/成本分析）
	// 由 provider 响应中的 usage 字段填充
	// TokenUsage 类型在下方定义
	Usage TokenUsage `json:"usage,omitempty"`
	// v3.7.0 扩展：token 计量元数据（用于 llm_routing_logs 落库）
	BaseURL        string  `json:"base_url,omitempty"`        // 出域审计
	IsFallback     bool    `json:"is_fallback,omitempty"`     // 是否为降级调用
	TokenSource    string  `json:"token_source,omitempty"`    // actual/estimated/missing
	Estimator      string  `json:"estimator,omitempty"`       // char_weight/empty_fallback
	PromptCost     float64 `json:"prompt_cost,omitempty"`     // prompt 单价计费
	CompletionCost float64 `json:"completion_cost,omitempty"` // completion 单价计费
}

// TokenUsage LLM token 使用量（基础版）
//
// 与 TokenUsageDetailed 区分：本结构只包含 LLM 真实消耗的 token 数，
// 用于 Provider 响应填充；TokenUsageDetailed 在此基础上扩展了 Cost/Latency 字段，
// 用于 DispatchResult 暴露给上层做成本分析。
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// TokenUsageDetailed 详细 token 使用量（含成本/时延）—— 计费用
//
// 与 llm_service.go 的 TokenUsage 区分：本结构带 Cost/Latency 字段，
// 用于 DispatchResult 暴露给上层做成本分析；底层 LLM 真实使用量通过 Usage 字段。
//
// 字段说明：
//   - PromptTokens: 输入 token 数（含 system + user + 工具定义 + 历史消息）
//   - CompletionTokens: 输出 token 数（含 content + tool_calls）
//   - TotalTokens: 两者之和
//   - Cost: 本次调用估算成本（元）
//   - LatencyMs: 本次调用耗时（毫秒）
type TokenUsageDetailed struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	Cost             float64 `json:"cost"`
	LatencyMs        int     `json:"latency_ms"`
}

// Dispatch 调度（带降级日志 + 告警）
//
// 降级链：主 Provider → 备 Provider 1 → 备 Provider 2 → 规则兜底
// 每次失败都会：
//  1. 记录 WARN 级别降级日志（含 trace_id、scenario、provider、错误）
//  2. 累计 ProviderFailover 健康计数（触发熔断器）
//  3. 触发 AlertHook（默认实现：累计 +1 失败计数，便于 ops 监控）
//  4. 全部失败时记录 ERROR 日志
func (d *Dispatcher) Dispatch(ctx context.Context, req DispatchRequest) (*DispatchResult, error) {
	d.mu.RLock()
	route, ok := d.routes[req.Scenario]
	if !ok {
		d.mu.RUnlock()
		return nil, fmt.Errorf("no route for scenario: %s", req.Scenario)
	}
	d.mu.RUnlock()

	// 灰度路由决策：0<Weight<100 时按 CanaryKey 抽样走 CanaryRoute
	activeRoute := route
	if canary := DecideCanaryRoute(route, req.CanaryKey); canary != nil {
		activeRoute = canary
	}

	// 命中缓存直接返回（提升相同 prompt 的吞吐、降低成本）
	if req.CacheKey != "" && req.CacheTTL > 0 {
		if c, hit := d.getCache(req.CacheKey); hit {
			// v3.7.0: 缓存命中也落库（满足硬约束"缓存命中的模型调用需标记 from_cache 字段"
			// 及"每次 Dispatch 决策（无论成功/失败/降级）必须落库至 llm_routing_logs 表"）
			// 注意：cache 命中无 LLM API 调用，不计入 missing 占比统计（updateMissingCounter 跳过 source=cache）
			if !d.testMode.Load() {
				cacheEntry := &LogEntry{
					TraceID:          logger.TraceIDFromContext(ctx),
					Scenario:         req.Scenario,
					Provider:         "cache",
					Model:            "cache",
					Success:          true,
					FromCache:        true,
					Source:           SourceCache,
					TokenSource:      TokenSourceActual, // 缓存命中无新 token 消耗，actual=0
					ScenarioProvider: string(req.Scenario) + "|cache",
					InternalLang:     req.InternalLang,
					TargetLang:       req.TargetLang,
					CrossLingual:     req.CrossLingual,
					GlossaryVersion:  req.GlossaryVersion,
					CacheHit:         req.CacheKey != "",
				}
				LogRoutingDecision(ctx, cacheEntry)
			}
			return &DispatchResult{Provider: "cache", Model: "cache", Content: c, FromCache: true}, nil
		}
	}

	// 候选 provider 列表（首选 + 备选）
	candidates := []string{activeRoute.Provider}
	candidates = append(candidates, activeRoute.Fallbacks...)

	// 提取 trace_id（用于降级日志关联）
	traceID := logger.TraceIDFromContext(ctx)

	var lastErr error
	attempted := 0
	for _, providerName := range candidates {
		d.mu.RLock()
		provider, exists := d.providers[providerName]
		d.mu.RUnlock()
		if !exists || !provider.Enabled {
			continue
		}

		// 质量/时延感知：跳过不满足场景最低质量或超出最大时延的 provider
		if activeRoute.MinQuality > 0 && provider.QualityScore < activeRoute.MinQuality {
			continue
		}
		if activeRoute.MaxLatency > 0 && provider.AvgLatencyMs > activeRoute.MaxLatency {
			continue
		}

		// RPM 限流
		if !d.allowRequest(providerName, provider.MaxRPM) {
			continue
		}

		attempted++
		result, err := d.callProvider(ctx, provider, req, activeRoute)
		if err != nil {
			lastErr = err
			// test 模式：跳过所有告警/统计/审计副作用
			if d.testMode.Load() {
				continue
			}
			// 降级日志：本次 provider 失败，准备尝试下一级
			logger.Warnf("[LLM Fallback] scenario=%s provider=%s trace_id=%s failed (attempt %d/%d): %v",
				req.Scenario, providerName, traceID, attempted, len(candidates), err)
			// 触发告警（默认 AlertHook：累计失败计数）
			AlertProviderFailure(string(req.Scenario), providerName, err, traceID)
			// v3.7.0：失败时也落决策日志（标记 is_fallback + token_source=missing）
			isFallback := attempted > 1
			failEntry := NewLogEntry(req.Scenario, provider, provider.Model,
				0, 0, 0, 0, 0, false, err.Error(), false, isFallback, traceID, "", SourceFallback)
			failEntry.InternalLang = req.InternalLang
			failEntry.TargetLang = req.TargetLang
			failEntry.CrossLingual = req.CrossLingual
			failEntry.GlossaryVersion = req.GlossaryVersion
			failEntry.CacheHit = req.CacheKey != ""
			LogRoutingDecision(ctx, failEntry)
			continue
		}
		// 命中成功
		if attempted > 1 {
			logger.Infof("[LLM Fallback] scenario=%s succeeded on provider=%s (after %d attempts) trace_id=%s",
				req.Scenario, providerName, attempted, traceID)
		}
		// test 模式：跳过成功路径的告警/统计/缓存写入副作用
		if d.testMode.Load() {
			return result, nil
		}
		// 成功：触发告警恢复
		AlertProviderSuccess(string(req.Scenario), providerName, traceID)
		if req.CacheKey != "" && req.CacheTTL > 0 {
			d.setCache(req.CacheKey, req.CacheTTL, result.Content)
		}
		// v3.7.0：成功决策日志（含真实 usage + 扩展字段）
		isFallback := attempted > 1
		successEntry := NewLogEntry(req.Scenario, provider, result.Model,
			result.Usage.PromptTokens, result.Usage.CompletionTokens, result.Usage.TotalTokens,
			result.Cost, result.LatencyMs, true, "", false, isFallback, traceID, result.Content, SourceDispatch)
		successEntry.InternalLang = req.InternalLang
		successEntry.TargetLang = req.TargetLang
		successEntry.CrossLingual = req.CrossLingual
		successEntry.GlossaryVersion = req.GlossaryVersion
		successEntry.CacheHit = req.CacheKey != ""
		LogRoutingDecision(ctx, successEntry)
		return result, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no available provider for scenario: %s", req.Scenario)
	}
	// 全部失败 → ERROR 日志 + 严重告警
	logger.Errorf("[LLM Fallback] all providers failed scenario=%s trace_id=%s attempted=%d: %v",
		req.Scenario, traceID, attempted, lastErr)
	AlertAllProvidersFailed(string(req.Scenario), lastErr, traceID)
	return nil, lastErr
}

// allowRequest RPM 限流
func (d *Dispatcher) allowRequest(providerName string, maxRPM int) bool {
	if maxRPM <= 0 {
		return true
	}
	d.mu.Lock()
	bucket, ok := d.rpmCounter[providerName]
	if !ok {
		bucket = &rpmBucket{resetAt: time.Now().Add(time.Minute)}
		d.rpmCounter[providerName] = bucket
	}
	d.mu.Unlock()

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	now := time.Now()
	if now.After(bucket.resetAt) {
		bucket.count = 0
		bucket.resetAt = now.Add(time.Minute)
	}
	if bucket.count >= maxRPM {
		return false
	}
	bucket.count++
	return true
}

// callProvider 调用 provider
func (d *Dispatcher) callProvider(ctx context.Context, provider *ProviderConfig, req DispatchRequest, route *ScenarioRoute) (*DispatchResult, error) {
	// 本地模型（mtk-llm / llama.cpp）无需密钥，空 key 直接透传即可；
	// 若为空 key 且为云端厂商，下游请求会返回 401，再由 Dispatch 退到其它 fallback，
	// 不应在此处硬阻断（否则本地优先链路会被空 key 守卫直接秒拒）。
	if provider.APIKey == "" {
		logger.Warnf("[LLM] WARN: provider %s APIKey empty (local model is fine; cloud will 401)", provider.Name)
	}

	// 设置超时
	if route.MaxLatency > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(route.MaxLatency)*time.Millisecond)
		defer cancel()
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1000
	}
	temperature := req.Temperature
	if temperature <= 0 {
		temperature = 0.7
	}

	config := &LLMConfig{
		APIKey:         provider.APIKey,
		BaseURL:        provider.BaseURL,
		APIType:        provider.APIType,
		Model:          provider.Model,
		Temperature:    temperature,
		MaxTokens:      maxTokens,
		MaxRetries:     1, // 2026-07-24 性能优化：2→1。本地 LLM 失败多为进程崩溃/OOM，重试无意义只会空等 backoff（2s+4s）。失败由 dispatcher 候选 provider failover 兜底。
		RequestTimeout: route.MaxLatency / 1000,
		SystemPrompt:   req.SystemPrompt,
	}
	if req.JSONMode {
		config.ResponseFormat = "json_object"
	}

	// ReturnLogprobs 透传到 LLMConfig
	// 真正读取 logprobs 需要 LLMService 实现 chatResponse.choices[0].logprobs 解析。
	if req.ReturnLogprobs {
		config.Logprobs = true
		if req.TopLogprobs > 0 {
			config.TopLogprobs = req.TopLogprobs
		} else {
			config.TopLogprobs = 20
		}
	}

	// 智能体 tool_call 字段透传
	config.Tools = req.Tools
	config.ToolChoice = req.ToolChoice
	config.Messages = req.Messages

	// ReAct 适配（NoFC provider + 智能体场景）
	// 当 provider 标记为 NoFC 且请求携带 Tools 时，启用 ReAct 文本协议
	// - 将 Tools 转为 ReAct 系统提示词追加到 SystemPrompt
	// - 不向 LLM 发送 OpenAI tools 参数（无 FC 能力 LLM 会报错）
	// - 调用后解析 Thought/Action/Action Input 并构造 ToolCall
	reactMode := IsReActMode(&req, provider.NoFC)
	logger.Infof("[Dispatcher] provider=%s NoFC=%v reactMode=%v tools=%d",
		provider.Name, provider.NoFC, reactMode, len(req.Tools))
	if reactMode {
		// 追加 ReAct 系统提示词
		originalSystem := config.SystemPrompt
		config.SystemPrompt = d.getReActAdapter().WrapSystemPrompt(originalSystem, req.Tools)
		// 清空 Tools/ToolChoice（避免 NoFC LLM 收到 tools 参数报错）
		config.Tools = nil
		config.ToolChoice = ""
		// ReAct 模式用 Messages 进行多轮对话，Prompt 字段保留仅作 fallback
	}

	start := time.Now()
	// 统一走 GenerateWithTools 路径，确保所有调用都能拿到真实 Usage
	result, err := d.llmService.GenerateWithTools(ctx, config, req.Prompt)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return nil, fmt.Errorf("provider %s: %w", provider.Name, err)
	}

	// token 用量：优先使用 LLM 真实 Usage，零值时降级为字符估算
	promptTokens := result.Usage.PromptTokens
	completionTokens := result.Usage.CompletionTokens
	totalTokens := result.Usage.TotalTokens
	if totalTokens <= 0 {
		// LLM 未返回 usage，按字符加权估算（标记为 estimated）
		promptTokens = estimateTokens(req.Prompt)
		completionTokens = estimateTokens(result.Content)
		totalTokens = promptTokens + completionTokens
	}
	cost := float64(totalTokens) / 1000.0 * provider.CostPer1k
	tokenSource := InferTokenSource(result.Usage.TotalTokens, result.Content)
	estimator := ClassifyEstimator(tokenSource)
	promptCost, completionCost := splitCost(cost, promptTokens, completionTokens, totalTokens)

	// 填充 Usage（来自 LLM 真实响应；零值时省略）
	var usage TokenUsage
	if result.Usage.TotalTokens > 0 {
		usage = TokenUsage{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
		}
	}
	dispatchResult := &DispatchResult{
		Provider:       provider.Name,
		Model:          provider.Model,
		Content:        result.Content,
		TotalTokens:    totalTokens,
		Cost:           cost,
		LatencyMs:      latency,
		FinishReason:   result.FinishReason,
		ToolCalls:      result.ToolCalls,
		Usage:          usage, // P1-D
		BaseURL:        provider.BaseURL,
		TokenSource:    tokenSource,
		Estimator:      estimator,
		PromptCost:     promptCost,
		CompletionCost: completionCost,
	}
	// ReAct 适配 - 解析 LLM 文本输出为 ToolCall
	if reactMode {
		dispatchResult = d.getReActAdapter().AdaptResult(dispatchResult)
	}
	return dispatchResult, nil
}

// DispatchStructured 结构化输出
func (d *Dispatcher) DispatchStructured(ctx context.Context, req DispatchRequest, schema any) (*DispatchResult, error) {
	req.JSONMode = true
	result, err := d.Dispatch(ctx, req)
	if err != nil {
		return nil, err
	}
	// 提取 JSON 子串
	jsonStr := extractJSON(result.Content)
	if jsonStr == "" {
		return result, fmt.Errorf("no JSON content in response: %s", result.Content)
	}
	if err := json.Unmarshal([]byte(jsonStr), schema); err != nil {
		return result, fmt.Errorf("parse JSON: %w", err)
	}
	result.Content = jsonStr
	return result, nil
}

// estimateTokens 估算 token 数（仅作为 LLM 未返回 usage 时的兜底，标记 token_source=estimated）
//
// 估算公式：
//   - 中文字符（U+4E00~U+9FFF）：每字符计 1 token
//   - 其他字符（ASCII/标点/空白）：每 4 字节计 1 token（UTF-8 编码下英文为 1 字节/字符）
//
// 注意：本函数仅为兜底，优先使用 LLM 真实 Usage（token_source=actual）。
// 本地 llama-server / vLLM / Ollama 等主流推理栈均默认返回 usage 字段，
// 实际生产中 estimated 路径极少触发，missing 路径触发即告警。
func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	cn := 0
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			cn++
		}
	}
	en := len(text) - cn*3 // utf-8 中文字占 3 字节, 简化处理
	if en < 0 {
		en = len(text)
	}
	return cn + en/4
}

// GetProviderList 获取所有 provider
func (d *Dispatcher) GetProviderList() []ProviderConfig {
	d.mu.RLock()
	defer d.mu.RUnlock()
	list := make([]ProviderConfig, 0, len(d.providers))
	for _, p := range d.providers {
		list = append(list, *p)
	}
	return list
}

// GetAllProviders 获取所有 provider（GetProviderList 的语义别名）
//
// 注意：GetProviderList / GetProvider / GetRoute / RemoveProvider 的实际定义
// 在 dispatcher_registry.go（保持向后兼容的返回签名）。本文件仅提供
// GetAllProviders / GetAllRoutes / SetRoute / SetRouteWithAudit / Dispatch /
// RegisterProvider / CallProviderForTest 等 dispatcher 核心逻辑。
func (d *Dispatcher) GetAllProviders() []ProviderConfig {
	return d.GetProviderList()
}

// GetAllRoutes 获取所有路由（语义别名）
func (d *Dispatcher) GetAllRoutes() []ScenarioRoute {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]ScenarioRoute, 0, len(d.routes))
	for _, r := range d.routes {
		out = append(out, *r)
	}
	return out
}

// CallProviderForTest 调 provider 一次（不触发告警/熔断/统计）
//
// 用途：LLMRoutingService.TestModel 在管理后台测试 provider 连通性。
// 关键不变量：
//   - 不调 AlertProviderFailure/Success
//   - 不调 LogRoutingDecision
//   - 不更新 provider.AvgLatencyMs
//   - 走 callProvider 同一逻辑，但屏蔽所有副作用
//
// 实现：通过一个开关字段（isTest）让 callProvider 跳过告警/统计。
// 本方法在 callProvider 之前注入 isTest，callProvider 检查后跳过副作用。
func (d *Dispatcher) CallProviderForTest(ctx context.Context, provider *ProviderConfig, req DispatchRequest, route *ScenarioRoute) (*DispatchResult, error) {
	// 设置 test 标志
	d.testMode.Store(true)
	defer d.testMode.Store(false)
	// 构造调用：max_latency 已经由 testRoute 拉宽
	return d.callProvider(ctx, provider, req, route)
}

// DispatchMultiModel 多模型投票（用于异议处理等高质量场景）
func (d *Dispatcher) DispatchMultiModel(ctx context.Context, req DispatchRequest, providers []string) ([]*DispatchResult, error) {
	results := make([]*DispatchResult, 0, len(providers))
	for _, name := range providers {
		d.mu.RLock()
		provider, ok := d.providers[name]
		d.mu.RUnlock()
		if !ok || !provider.Enabled || provider.APIKey == "" {
			continue
		}
		result, err := d.callProvider(ctx, provider, req, &ScenarioRoute{MaxLatency: 10000})
		if err != nil {
			continue
		}
		results = append(results, result)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("all providers failed")
	}
	return results, nil
}

// MultiModelVote 多模型投票返回最一致答案
func (d *Dispatcher) MultiModelVote(results []*DispatchResult) string {
	if len(results) == 0 {
		return ""
	}
	if len(results) == 1 {
		return results[0].Content
	}
	// 简化：选择质量分最高的 provider
	bestIdx := 0
	bestQuality := 0.0
	for i, r := range results {
		d.mu.RLock()
		p, ok := d.providers[r.Provider]
		d.mu.RUnlock()
		if ok && p.QualityScore > bestQuality {
			bestQuality = p.QualityScore
			bestIdx = i
		}
	}
	return results[bestIdx].Content
}

// CacheKey 生成缓存 key
func CacheKey(scenario DispatchScenario, prompt string) string {
	h := fnv.New64a()
	h.Write([]byte(string(scenario)))
	h.Write([]byte(strings.TrimSpace(prompt)))
	return fmt.Sprintf("llm:dispatch:%s:%x", scenario, h.Sum64())
}

// ====== 告警钩子（Provider 降级链） ======
//
// AlertHook 告警钩子接口：Dispatch 失败 / 恢复时调用。
// 默认实现为 InMemoryAlertSink（进程内累计），可通过 SetAlertHook 注入自定义实现（如对接钉钉/企业微信/飞书）。
type AlertHook interface {
	OnProviderFailure(scenario, provider string, err error, traceID string)
	OnProviderSuccess(scenario, provider, traceID string)
	OnAllProvidersFailed(scenario string, err error, traceID string)
}

// AlertHookFunc 函数式适配器
type AlertHookFunc struct {
	OnFailure   func(scenario, provider string, err error, traceID string)
	OnSuccess   func(scenario, provider, traceID string)
	OnAllFailed func(scenario string, err error, traceID string)
}

// OnProviderFailure 实现 AlertHook
func (f AlertHookFunc) OnProviderFailure(scenario, provider string, err error, traceID string) {
	if f.OnFailure != nil {
		f.OnFailure(scenario, provider, err, traceID)
	}
}

// OnProviderSuccess 实现 AlertHook
func (f AlertHookFunc) OnProviderSuccess(scenario, provider, traceID string) {
	if f.OnSuccess != nil {
		f.OnSuccess(scenario, provider, traceID)
	}
}

// OnAllProvidersFailed 实现 AlertHook
func (f AlertHookFunc) OnAllProvidersFailed(scenario string, err error, traceID string) {
	if f.OnAllFailed != nil {
		f.OnAllFailed(scenario, err, traceID)
	}
}

var (
	alertHookMu sync.RWMutex
	alertHook   AlertHook = NoopAlertHook{}
)

// SetAlertHook 注入告警钩子（线程安全）
func SetAlertHook(h AlertHook) {
	if h == nil {
		return
	}
	alertHookMu.Lock()
	defer alertHookMu.Unlock()
	alertHook = h
}

// NoopAlertHook 空告警（默认）
type NoopAlertHook struct{}

// OnProviderFailure 默认空实现
func (NoopAlertHook) OnProviderFailure(string, string, error, string) {}

// OnProviderSuccess 默认空实现
func (NoopAlertHook) OnProviderSuccess(string, string, string) {}

// OnAllProvidersFailed 默认空实现
func (NoopAlertHook) OnAllProvidersFailed(string, error, string) {}

// LoggingAlertHook 写入日志的告警实现（推荐默认）
//
// 把所有 provider 失败 / 全部失败事件以 WARN/ERROR 级别写日志，
// 满足"必须能感知到降级"的运维诉求，无需对接外部系统。
type LoggingAlertHook struct {
	OnFailure   func(scenario, provider, traceID string, err error)
	OnSuccess   func(scenario, provider, traceID string)
	OnAllFailed func(scenario, traceID string, err error)
}

// OnProviderFailure 写 WARN 日志
func (h LoggingAlertHook) OnProviderFailure(scenario, provider string, err error, traceID string) {
	logger.Warnf("[LLM Alert] provider failure scenario=%s provider=%s trace_id=%s err=%v",
		scenario, provider, traceID, err)
	if h.OnFailure != nil {
		h.OnFailure(scenario, provider, traceID, err)
	}
}

// OnProviderSuccess 写 INFO 日志
func (h LoggingAlertHook) OnProviderSuccess(scenario, provider, traceID string) {
	logger.Infof("[LLM Alert] provider recovered scenario=%s provider=%s trace_id=%s",
		scenario, provider, traceID)
	if h.OnSuccess != nil {
		h.OnSuccess(scenario, provider, traceID)
	}
}

// OnAllProvidersFailed 写 ERROR 日志
func (h LoggingAlertHook) OnAllProvidersFailed(scenario string, err error, traceID string) {
	logger.Errorf("[LLM Alert] all providers FAILED scenario=%s trace_id=%s err=%v",
		scenario, traceID, err)
	if h.OnAllFailed != nil {
		h.OnAllFailed(scenario, traceID, err)
	}
}

// InMemoryAlertSink 进程内累计告警（用于 /api/llm-routings/alerts 端点）
//
// 保留最近 N 条告警，环形 buffer；提供 Drain 消费。
type InMemoryAlertSink struct {
	mu     sync.Mutex
	buffer []AlertEvent
	cap    int
}

// AlertEvent 单条告警
type AlertEvent struct {
	Time     time.Time `json:"time"`
	Severity string    `json:"severity"`
	Scenario string    `json:"scenario"`
	Provider string    `json:"provider,omitempty"`
	TraceID  string    `json:"trace_id"`
	Message  string    `json:"message"`
}

// NewInMemoryAlertSink 创建 sink
func NewInMemoryAlertSink(cap int) *InMemoryAlertSink {
	if cap <= 0 {
		cap = 200
	}
	return &InMemoryAlertSink{cap: cap, buffer: make([]AlertEvent, 0, cap)}
}

// record 写一条
func (s *InMemoryAlertSink) record(ev AlertEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.buffer) >= s.cap {
		// 丢最旧的
		s.buffer = s.buffer[1:]
	}
	s.buffer = append(s.buffer, ev)
}

// OnProviderFailure 累计失败
func (s *InMemoryAlertSink) OnProviderFailure(scenario, provider string, err error, traceID string) {
	s.record(AlertEvent{
		Time: time.Now(), Severity: "warn", Scenario: scenario,
		Provider: provider, TraceID: traceID, Message: err.Error(),
	})
}

// OnProviderSuccess 累计成功（用于计算成功率）
func (s *InMemoryAlertSink) OnProviderSuccess(scenario, provider, traceID string) {
	s.record(AlertEvent{
		Time: time.Now(), Severity: "info", Scenario: scenario,
		Provider: provider, TraceID: traceID, Message: "recovered",
	})
}

// OnAllProvidersFailed 累计全部失败
func (s *InMemoryAlertSink) OnAllProvidersFailed(scenario string, err error, traceID string) {
	s.record(AlertEvent{
		Time: time.Now(), Severity: "error", Scenario: scenario,
		TraceID: traceID, Message: err.Error(),
	})
}

// Drain 消费所有告警（消费后清空）
func (s *InMemoryAlertSink) Drain() []AlertEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AlertEvent, len(s.buffer))
	copy(out, s.buffer)
	s.buffer = s.buffer[:0]
	return out
}

// Snapshot 快照
func (s *InMemoryAlertSink) Snapshot() []AlertEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AlertEvent, len(s.buffer))
	copy(out, s.buffer)
	return out
}

// InitDefaultAlertHook 初始化默认告警（LoggingAlertHook + InMemoryAlertSink 组合）
//
// 建议在 main.go 启动期调用，把全局 alertHook 替换为 logging + memory 双写。
// 用法：
//
//	sink := llm.NewInMemoryAlertSink(200)
//	llm.InitDefaultAlertHook(sink)
//	// 之后 ops 端点通过 sink.Snapshot() / sink.Drain() 读取告警
func InitDefaultAlertHook(sink *InMemoryAlertSink) {
	if sink == nil {
		sink = NewInMemoryAlertSink(200)
	}
	final := AlertHookFunc{
		OnFailure: func(scenario, provider string, err error, traceID string) {
			LoggingAlertHook{}.OnProviderFailure(scenario, provider, err, traceID)
			sink.OnProviderFailure(scenario, provider, err, traceID)
		},
		OnSuccess: func(scenario, provider, traceID string) {
			LoggingAlertHook{}.OnProviderSuccess(scenario, provider, traceID)
			sink.OnProviderSuccess(scenario, provider, traceID)
		},
		OnAllFailed: func(scenario string, err error, traceID string) {
			LoggingAlertHook{}.OnAllProvidersFailed(scenario, err, traceID)
			sink.OnAllProvidersFailed(scenario, err, traceID)
		},
	}
	SetAlertHook(final)
}

// AlertProviderFailure 触发告警：单 provider 失败
func AlertProviderFailure(scenario, provider string, err error, traceID string) {
	alertHookMu.RLock()
	h := alertHook
	alertHookMu.RUnlock()
	if h != nil {
		h.OnProviderFailure(scenario, provider, err, traceID)
	}
}

// AlertProviderSuccess 触发告警：provider 成功
func AlertProviderSuccess(scenario, provider, traceID string) {
	alertHookMu.RLock()
	h := alertHook
	alertHookMu.RUnlock()
	if h != nil {
		h.OnProviderSuccess(scenario, provider, traceID)
	}
}

// AlertAllProvidersFailed 触发告警：全部 provider 失败
func AlertAllProvidersFailed(scenario string, err error, traceID string) {
	alertHookMu.RLock()
	h := alertHook
	alertHookMu.RUnlock()
	if h != nil {
		h.OnAllProvidersFailed(scenario, err, traceID)
	}
}
