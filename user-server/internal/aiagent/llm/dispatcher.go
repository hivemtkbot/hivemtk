package llm

import (
	"context"

	"fmt"

	"sort"

	"strings"

	"sync"

	"sync/atomic"

	"time"

	"hivemtk-user/internal/cache"

	"hivemtk-user/internal/config"

	"hivemtk-user/internal/pkg/utils/logger"
)

type DispatchScenario string

const (
	ScenarioIntentRecognize DispatchScenario = "intent_recognize" // 意图识别

	ScenarioSOPReply DispatchScenario = "sop_reply" // SOP 销冠回复

	ScenarioObjection DispatchScenario = "objection" // 异议处理

	ScenarioFriendlyChat DispatchScenario = "friendly_chat" // 拟人寒暄

	ScenarioLongSummary DispatchScenario = "long_summary" // 长文本总结

	ScenarioHighQuality DispatchScenario = "high_quality" // 高质量回复

	ScenarioLowCost DispatchScenario = "low_cost" // 低成本批量

)

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
	// DisplayName/Vendor/Tags 为可视化展示与分类元数据，随 provider 落库
	DisplayName string   `json:"display_name,omitempty"`
	Vendor      string   `json:"vendor,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

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

type Dispatcher struct {
	mu         sync.RWMutex
	providers  map[string]*ProviderConfig
	routes     map[DispatchScenario]*ScenarioRoute
	llmService *LLMService
	rpmCounter map[string]*rpmBucket
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

func newDispatcherBase(llmService *LLMService) *Dispatcher {
	return &Dispatcher{
		providers:  make(map[string]*ProviderConfig),
		routes:     make(map[DispatchScenario]*ScenarioRoute),
		llmService: llmService,
		rpmCounter: make(map[string]*rpmBucket),
	}
}

func (d *Dispatcher) getReActAdapter() *ReActAdapter {
	d.reactAdapterMu.Do(func() {
		d.reactAdapter = NewReActAdapter()
	})
	return d.reactAdapter
}

func NewDispatcher(llmService *LLMService) *Dispatcher {
	d := newDispatcherBase(llmService)
	d.registerDefaultProviders()
	d.registerDefaultRoutes()
	return d
}

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
	d.registerLocalFirstRoutes(cfg.Inference.LLM.PrimaryProvider, timeoutSec*1000) // ms
	return d
}

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

func resolveNoFC(llmCfg config.InferenceLLMConfig) bool {
	// 用户显式设置 no_fc 时，直接使用该值（跳过 URL 启发式）
	if llmCfg.NoFC != nil {
		return *llmCfg.NoFC
	}
	// URL 启发式兜底（用户未设置时）
	base := strings.ToLower(llmCfg.BaseURL)
	return base == "" || strings.Contains(base, "127.0.0.1") || strings.Contains(base, "localhost") || strings.Contains(base, "mtk-llm")
}

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
	}
}

func (d *Dispatcher) registerCloudProvidersFromConfig(llmCfg config.InferenceLLMConfig) {
	// 1) 占位注册：deepseek 默认 disabled（测试用）
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
			// 云端强模型质量分设为 0.96，确保通过 high_quality(0.95)/objection(0.92) 等场景门槛
			QualityScore: 0.96,
			MaxRPM:       60,
			Enabled:      enabled,
		}
	}
}

func (d *Dispatcher) registerLocalFirstRoutes(primary string, maxLatencyMs int) {
	if maxLatencyMs <= 0 {
		maxLatencyMs = 180000
	}
	// 主 provider：默认走本地 "default"；若配置了 primary_provider（如云端 deepseek），
	// 则以其为主、本地作为兜底 fallback（满足"暂时用云端代替本地"的部署切换需求，无需改代码）。
	prim := "default"
	fallback := []string{}
	if primary != "" && primary != "default" {
		prim = primary
		fallback = []string{"default"}
	}
	routes := []*ScenarioRoute{
		{Scenario: ScenarioIntentRecognize, Provider: prim, Fallbacks: fallback, CostWeight: 5, MaxLatency: maxLatencyMs, MinQuality: 0.8},
		{Scenario: ScenarioSOPReply, Provider: prim, Fallbacks: fallback, CostWeight: 2, MaxLatency: maxLatencyMs, MinQuality: 0.9},
		{Scenario: ScenarioObjection, Provider: prim, Fallbacks: fallback, CostWeight: 1, MaxLatency: maxLatencyMs, MinQuality: 0.92},
		{Scenario: ScenarioFriendlyChat, Provider: prim, Fallbacks: fallback, CostWeight: 10, MaxLatency: maxLatencyMs, MinQuality: 0.8},
		{Scenario: ScenarioLongSummary, Provider: prim, Fallbacks: fallback, CostWeight: 3, MaxLatency: maxLatencyMs, MinQuality: 0.85},
		{Scenario: ScenarioHighQuality, Provider: prim, Fallbacks: fallback, CostWeight: 1, MaxLatency: maxLatencyMs, MinQuality: 0.95},
		{Scenario: ScenarioLowCost, Provider: prim, Fallbacks: fallback, CostWeight: 15, MaxLatency: maxLatencyMs, MinQuality: 0.7},
	}
	for _, r := range routes {
		d.routes[r.Scenario] = r
	}
}

func (d *Dispatcher) getCache(key string) (string, bool) {
	if key == "" {
		return "", false
	}
	raw, err := cache.GetGlobalCache().Get(context.Background(), key)
	if err != nil || raw == "" {
		return "", false
	}
	return raw, true
}

func (d *Dispatcher) setCache(key string, ttl int, content string) {
	if ttl <= 0 || key == "" {
		return
	}
	_ = cache.GetGlobalCache().Set(context.Background(), key, content, time.Duration(ttl)*time.Second)
}

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

func (d *Dispatcher) AddProvider(p ProviderConfig) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.providers[p.Name] = &p
}

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

func (d *Dispatcher) SetRouteWithAudit(ctx context.Context, r ScenarioRoute, action, operator, traceID string) ScenarioRoute {
	prev := d.GetRoute(r.Scenario)
	applied := d.SetRoute(r)
	// 路由本体落库：覆盖代码种子，重启不丢、多实例一致（db 未就绪时静默跳过）
	if err := d.UpsertRouteToDB(applied); err != nil {
		logger.Errorf("[LLM] SetRouteWithAudit 路由落库失败 scenario=%s: %v", r.Scenario, err)
	}
	d.writeAuditLog(ctx, r.Scenario, prev, &applied, action, operator, traceID)
	return applied
}

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
	// 多语言方案：跨语言生成元数据（由 service/translation 层注入）
	InternalLang    string `json:"internal_lang,omitempty"`    // 商户内部语言（知识库语言）
	TargetLang      string `json:"target_lang,omitempty"`      // 对外输出语言
	CrossLingual    bool   `json:"cross_lingual,omitempty"`    // 是否跨语言生成（InternalLang != TargetLang）
	GlossaryVersion string `json:"glossary_version,omitempty"` // 术语表版本（落库审计）
}

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

type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type TokenUsageDetailed struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	Cost             float64 `json:"cost"`
	LatencyMs        int     `json:"latency_ms"`
}

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

		// 集群熔断：该 provider 已被熔断（跨实例共享信号）则直接跳过，快速失败走兜底
		if fo := GetGlobalFailover(); fo != nil && fo.IsCircuitOpen(providerName) {
			logger.Debugf("[LLM] provider=%s 集群熔断中，跳过 scenario=%s", providerName, req.Scenario)
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
			// 喂给集群熔断器：真实失败累计，可跨实例触发/传播熔断信号
			if fo := GetGlobalFailover(); fo != nil {
				fo.RecordFailure(providerName, err)
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
		// 喂给集群熔断器：成功清零连续失败计数，并删除跨实例熔断信号
		if fo := GetGlobalFailover(); fo != nil {
			fo.RecordSuccess(providerName, int64(result.LatencyMs))
		}
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
	// 全局兜底：若路由首选+备选 provider 全部被跳过（禁用/熔断/限流/质量门禁），
	// 自动回退到「任意已启用且通过质量门禁的 provider（按质量分降序）」，
	// 保证"仅启用某个云端模型、关闭其它"的场景无需改路由表即可自动路由
	// （例：llm_routing_rules 为空、路由默认指向已禁用的 primary_provider 时，
	// 启用的云端模型仍能正常承接对话）。仅在没有任何候选被真正调用过
	// (attempted==0) 时才触发，避免正常失败重试被误判为"无候选"。
	if attempted == 0 {
		if fb := d.pickEnabledFallback(activeRoute); fb != "" {
			d.mu.RLock()
			provider, exists := d.providers[fb]
			d.mu.RUnlock()
			if exists {
				logger.Infof("[LLM] scenario=%s 路由候选全部不可用，兜底启用 provider=%s trace_id=%s", req.Scenario, fb, traceID)
				result, err := d.callProvider(ctx, provider, req, activeRoute)
				lastErr = err
				if err == nil {
					// 成功：记录决策日志 + 触发告警恢复
					if fo := GetGlobalFailover(); fo != nil {
						fo.RecordSuccess(fb, int64(result.LatencyMs))
					}
					AlertProviderSuccess(string(req.Scenario), fb, traceID)
					if req.CacheKey != "" && req.CacheTTL > 0 {
						d.setCache(req.CacheKey, req.CacheTTL, result.Content)
					}
					fallbackEntry := NewLogEntry(req.Scenario, provider, result.Model,
						result.Usage.PromptTokens, result.Usage.CompletionTokens, result.Usage.TotalTokens,
						result.Cost, result.LatencyMs, true, "", false, true, traceID, result.Content, SourceDispatch)
					fallbackEntry.InternalLang = req.InternalLang
					fallbackEntry.TargetLang = req.TargetLang
					fallbackEntry.CrossLingual = req.CrossLingual
					fallbackEntry.GlossaryVersion = req.GlossaryVersion
					fallbackEntry.CacheHit = req.CacheKey != ""
					LogRoutingDecision(ctx, fallbackEntry)
					return result, nil
				}
				logger.Warnf("[LLM] scenario=%s 兜底 provider=%s 调用失败: %v", req.Scenario, fb, lastErr)
				AlertProviderFailure(string(req.Scenario), fb, lastErr, traceID)
			}
		}
	}

	if lastErr == nil {
		// 所有候选均被跳过（熔断/限流/质量门禁/禁用），未发起任何真实请求：
		// 优雅降级返回模板话术，避免向调用方抛出硬错误（HA 需要）。
		logger.Warnf("[LLM] scenario=%s 无可用 provider（全部被熔断/限流跳过），返回降级回复 trace_id=%s", req.Scenario, traceID)
		return degradedReply(req), nil
	}
	// 全部失败 → ERROR 日志 + 严重告警
	logger.Errorf("[LLM Fallback] all providers failed scenario=%s trace_id=%s attempted=%d: %v",
		req.Scenario, traceID, attempted, lastErr)
	AlertAllProvidersFailed(string(req.Scenario), lastErr, traceID)
	return nil, lastErr
}

func (d *Dispatcher) pickEnabledFallback(route *ScenarioRoute) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	type pe struct {
		name  string
		score float64
	}
	list := make([]pe, 0, len(d.providers))
	for name, p := range d.providers {
		if !p.Enabled {
			continue
		}
		if route != nil && route.MinQuality > 0 && p.QualityScore < route.MinQuality {
			continue
		}
		list = append(list, pe{name: name, score: p.QualityScore})
	}
	if len(list) == 0 {
		return ""
	}
	sort.Slice(list, func(i, j int) bool { return list[i].score > list[j].score })
	return list[0].name
}
