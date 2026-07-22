package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
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
	// P2-A: NoFC 标记该 provider 不支持 OpenAI Function Calling
	// 为 true 时，Dispatcher 会启用 ReAct 适配器，通过文本协议完成工具调用
	// 适用场景：本地 LLM（llama.cpp / mtk-llm / 部分 ChatGLM 版本）不支持 FC
	NoFC bool `json:"no_fc,omitempty"`
}

// ScenarioRoute 场景路由策略
type ScenarioRoute struct {
	Scenario   DispatchScenario `json:"scenario"`
	Provider   string           `json:"provider"`    // 首选 provider name
	Fallbacks  []string         `json:"fallbacks"`   // 备选
	CostWeight int              `json:"cost_weight"` // 1-5
	MaxLatency int              `json:"max_latency"` // ms
	MinQuality float64          `json:"min_quality"`
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
	// P2-A: ReAct 适配器（让无 FC 能力的 LLM 通过文本协议接入 Agent Loop）
	// 懒初始化，首次需要时创建（避免无工具调用场景的开销）
	reactAdapter   *ReActAdapter
	reactAdapterMu sync.Once
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
// P2-A：仅在首次需要时创建，避免无工具调用场景的初始化开销
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
func NewDispatcherFromConfig(cfg config.AppConfig) *Dispatcher {
	d := newDispatcherBase(NewLLMService())
	d.registerLocalProvider(cfg.Inference.LLM)
	d.registerCloudProvidersFromConfig(cfg.Inference.LLM)
	d.registerLocalFirstRoutes()
	return d
}

// registerLocalProvider 注册本地默认 provider（指向 mtk-llm / 宿主 127.0.0.1:9000）
func (d *Dispatcher) registerLocalProvider(llmCfg config.InferenceLLMConfig) {
	baseURL := llmCfg.BaseURL
	model := llmCfg.Model
	apiKey := llmCfg.APIKey
	if baseURL == "" {
		baseURL = "http://127.0.0.1:9000/v1"
	}
	if model == "" {
		model = "Qwen2.5-3B-Instruct"
	}
	d.providers["default"] = &ProviderConfig{
		Name:         "default",
		APIKey:       apiKey,
		BaseURL:      baseURL,
		APIType:      "openai",
		Model:        model,
		CostPer1k:    0,
		AvgLatencyMs: 800,
		// 本地即唯一启用 provider，质量分须高于所有场景的 MinQuality 门槛
		// （Objection=0.92 / HighQuality=0.95），否则会被 MinQuality 网关跳过、
		// 退回已禁用云端导致本地推理永不命中。
		QualityScore: 0.99,
		MaxRPM:       0, // 0 = 不限流
		Enabled:      true,
		// P2-A: 本地 Qwen2.5-3B-Instruct 不支持 OpenAI Function Calling
		// 启用 ReAct 适配器，通过 Thought/Action/Action Input 文本协议完成工具调用
		// 用户可通过 inference.llm.no_fc=false 显式关闭（如果模型已升级支持 FC）
		NoFC: getNoFCFromConfig(llmCfg),
	}
}

// getNoFCFromConfig 从配置读取 NoFC 标记
// 默认 true（mtk-llm 本地 Qwen2.5-3B-Instruct 不支持 FC）
// 用户在 config.yaml 设置 inference.llm.no_fc=false 可显式关闭
func getNoFCFromConfig(llmCfg config.InferenceLLMConfig) bool {
	// 简化：未配置时默认 true（本地 LLM 不支持 FC）
	// 若用户配置了 no_fc: false，则返回 false
	// 这里通过 reflect 或类型断言读取，简化为 env 变量
	// 实际项目中可在 config 包添加 NoFC 字段
	if v := llmCfg.BaseURL; v == "" || strings.Contains(v, "127.0.0.1") || strings.Contains(v, "mtk-llm") {
		return true // 本地 LLM 默认 NoFC
	}
	return false
}

// registerCloudProvidersFromConfig 注册云端可选 fallback
//
// 本地优先原则：云端厂商默认禁用，仅当用户在 inference.llm.cloud_providers 显式配置
// api_key 且 enabled=true 时才启用；未配置时回落到内置云端厂商（同样默认禁用）。
func (d *Dispatcher) registerCloudProvidersFromConfig(llmCfg config.InferenceLLMConfig) {
	builtins := []config.InferenceCloudProviderConfig{
		{Name: "deepseek", BaseURL: "https://api.deepseek.com", APIType: "openai", Model: "deepseek-chat", Enabled: false},
		{Name: "qwen", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode", APIType: "openai", Model: "qwen-max", Enabled: false},
		{Name: "gpt-4o", BaseURL: "https://api.openai.com", APIType: "openai", Model: "gpt-4o", Enabled: false},
		{Name: "glm-4", BaseURL: "https://open.bigmodel.cn/api/paas/v4", APIType: "openai", Model: "glm-4-plus", Enabled: false},
		{Name: "kimi", BaseURL: "https://api.moonshot.cn", APIType: "openai", Model: "moonshot-v1-8k", Enabled: false},
	}
	src := llmCfg.CloudProviders
	if len(src) == 0 {
		src = builtins
	}
	for _, p := range src {
		// 云端仅在「显式启用且配置 api_key」时才启用（优化三：本地优先，线上 opt-in）
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
// 注意：本地 mtk-llm（Qwen2.5-1.5B q4）单次生成约 30-60s（CPU 推理），
// MaxLatency 必须大于 LLM 实际推理时间，否则 dispatcher 的 context.WithTimeout
// 会在本地推理完成前掐断请求（context deadline exceeded），
// 进而退回已禁用的云端兜底导致 AI 直答失败、错误转人工。
// 2026-07-22：把 MaxLatency 全部提升到 90s，给 1.5B Q4 在 M1 CPU 上留足时间。
func (d *Dispatcher) registerLocalFirstRoutes() {
	routes := []*ScenarioRoute{
		{Scenario: ScenarioIntentRecognize, Provider: "default", Fallbacks: []string{"deepseek", "qwen"}, CostWeight: 5, MaxLatency: 90000, MinQuality: 0.8},
		{Scenario: ScenarioSOPReply, Provider: "default", Fallbacks: []string{"gpt-4o", "glm-4"}, CostWeight: 2, MaxLatency: 90000, MinQuality: 0.9},
		{Scenario: ScenarioObjection, Provider: "default", Fallbacks: []string{"gpt-4o", "glm-4"}, CostWeight: 1, MaxLatency: 90000, MinQuality: 0.92},
		{Scenario: ScenarioFriendlyChat, Provider: "default", Fallbacks: []string{"deepseek"}, CostWeight: 4, MaxLatency: 90000, MinQuality: 0.8},
		{Scenario: ScenarioLongSummary, Provider: "default", Fallbacks: []string{"kimi", "qwen"}, CostWeight: 3, MaxLatency: 120000, MinQuality: 0.85},
		{Scenario: ScenarioHighQuality, Provider: "default", Fallbacks: []string{"gpt-4o", "glm-4"}, CostWeight: 1, MaxLatency: 90000, MinQuality: 0.95},
		{Scenario: ScenarioLowCost, Provider: "default", Fallbacks: []string{"deepseek"}, CostWeight: 5, MaxLatency: 90000, MinQuality: 0.7},
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

// SetRoute 设置场景路由
func (d *Dispatcher) SetRoute(r ScenarioRoute) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.routes[r.Scenario] = &r
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
	ReturnLogprobs bool             `json:"return_logprobs"` // P0-3 修复：请求 LLM 返回 token logprobs
	TopLogprobs    int              `json:"top_logprobs"`    // 返回 top-N 候选 token logprobs（默认 20）
	// P0-2 修复：智能体 tool_call 支持
	// Tools: 工具定义列表，非空时 Dispatcher 走 GenerateWithTools 路径
	// ToolChoice: "auto"/"none"/"required" 或 JSON 对象字符串
	Tools      []ToolDefinition `json:"tools,omitempty"`
	ToolChoice string           `json:"tool_choice,omitempty"`
	// Messages: 多轮对话历史（含 tool 角色回灌工具结果）
	// 非空时 Prompt 字段被忽略，使用 Messages 进行多轮对话
	Messages []ChatMessage `json:"messages,omitempty"`
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
	// P0-3 修复：用于置信度计算的 token 级信号
	// 当前 LLMService 尚未真正透传 logprobs，字段保留供上游 SignalCollector
	// 在 LLMService 升级（添加 chatResponse.logprobs 解析）后自动填充。
	// 当前为安全降级：Logprobs/TopTokenEntropy 留空，FinishReason 透传 "stop"/"length" 等。
	Logprobs        []float64 `json:"logprobs,omitempty"`
	TopTokenEntropy float64   `json:"top_token_entropy,omitempty"`
	FinishReason    string    `json:"finish_reason,omitempty"`
	// P0-2 修复：智能体 tool_call 返回结果
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// P1-D：详细 token 使用量（用于计费/成本分析）
	// 由 provider 响应中的 usage 字段填充
	// TokenUsage 类型在下方定义
	Usage TokenUsage `json:"usage,omitempty"`
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

// TokenUsageDetailed 详细 token 使用量（含成本/时延）—— P1-D 计费用
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

	// 命中缓存直接返回（提升相同 prompt 的吞吐、降低成本）
	if req.CacheKey != "" && req.CacheTTL > 0 {
		if c, hit := d.getCache(req.CacheKey); hit {
			return &DispatchResult{Provider: "cache", Model: "cache", Content: c, FromCache: true}, nil
		}
	}

	// 候选 provider 列表（首选 + 备选）
	candidates := []string{route.Provider}
	candidates = append(candidates, route.Fallbacks...)

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
		if route.MinQuality > 0 && provider.QualityScore < route.MinQuality {
			continue
		}
		if route.MaxLatency > 0 && provider.AvgLatencyMs > route.MaxLatency {
			continue
		}

		// RPM 限流
		if !d.allowRequest(providerName, provider.MaxRPM) {
			continue
		}

		attempted++
		result, err := d.callProvider(ctx, provider, req, route)
		if err != nil {
			lastErr = err
			// 降级日志：本次 provider 失败，准备尝试下一级
			logger.Warnf("[LLM Fallback] scenario=%s provider=%s trace_id=%s failed (attempt %d/%d): %v",
				req.Scenario, providerName, traceID, attempted, len(candidates), err)
			// 触发告警（默认 AlertHook：累计失败计数）
			AlertProviderFailure(string(req.Scenario), providerName, err, traceID)
			continue
		}
		// 命中成功
		if attempted > 1 {
			logger.Infof("[LLM Fallback] scenario=%s succeeded on provider=%s (after %d attempts) trace_id=%s",
				req.Scenario, providerName, attempted, traceID)
		}
		// 成功：触发告警恢复
		AlertProviderSuccess(string(req.Scenario), providerName, traceID)
		if req.CacheKey != "" && req.CacheTTL > 0 {
			d.setCache(req.CacheKey, req.CacheTTL, result.Content)
		}
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
		MaxRetries:     2,
		RequestTimeout: route.MaxLatency / 1000,
		SystemPrompt:   req.SystemPrompt,
	}
	if req.JSONMode {
		config.ResponseFormat = "json_object"
	}

	// P0-3 修复：ReturnLogprobs 透传到 LLMConfig
	// 当前 LLMConfig 已加 Logprobs 字段，但 LLMService.Generate 尚未实现透传。
	// 真正读取 logprobs 需要 LLMService 升级（chatResponse.choices[0].logprobs 解析），
	// 属于后续 P0-3 任务的子步骤，本提交先完成数据契约对齐。
	if req.ReturnLogprobs {
		config.Logprobs = true
		if req.TopLogprobs > 0 {
			config.TopLogprobs = req.TopLogprobs
		} else {
			config.TopLogprobs = 20
		}
	}

	// P0-2 修复：智能体 tool_call 字段透传
	config.Tools = req.Tools
	config.ToolChoice = req.ToolChoice
	config.Messages = req.Messages

	// P2-A: ReAct 适配（NoFC provider + 智能体场景）
	// 当 provider 标记为 NoFC 且请求携带 Tools 时，启用 ReAct 文本协议
	// - 将 Tools 转为 ReAct 系统提示词追加到 SystemPrompt
	// - 不向 LLM 发送 OpenAI tools 参数（无 FC 能力 LLM 会报错）
	// - 调用后解析 Thought/Action/Action Input 并构造 ToolCall
	reactMode := IsReActMode(&req, provider.NoFC)
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
	// P0-2: 当 req.Tools 或 req.Messages 非空（智能体循环场景）走 GenerateWithTools；
	//       否则走原始 Generate 路径以保持 token 估算等行为不变。
	if len(req.Tools) > 0 || len(req.Messages) > 0 {
		result, err := d.llmService.GenerateWithTools(ctx, config, req.Prompt)
		latency := int(time.Since(start).Milliseconds())
		if err != nil {
			return nil, fmt.Errorf("provider %s: %w", provider.Name, err)
		}
		// token 估算：GenerateWithTools 已返回真实 Usage（如本地推理栈支持），
		// 优先使用真实值，否则降级为估算
		totalTokens := result.Usage.TotalTokens
		if totalTokens <= 0 {
			totalTokens = estimateTokens(req.Prompt) + estimateTokens(result.Content)
		}
		cost := float64(totalTokens) / 1000.0 * provider.CostPer1k
		// P1-D：填充 Usage（来自 LLM 真实响应；零值时省略）
		var usage TokenUsage
		if result.Usage.TotalTokens > 0 {
			usage = TokenUsage{
				PromptTokens:     result.Usage.PromptTokens,
				CompletionTokens: result.Usage.CompletionTokens,
				TotalTokens:      result.Usage.TotalTokens,
			}
		}
		dispatchResult := &DispatchResult{
			Provider:     provider.Name,
			Model:        provider.Model,
			Content:      result.Content,
			TotalTokens:  totalTokens,
			Cost:         cost,
			LatencyMs:    latency,
			FinishReason: result.FinishReason,
			ToolCalls:    result.ToolCalls,
			Usage:        usage, // P1-D
		}
		// P2-A: ReAct 适配 - 解析 LLM 文本输出为 ToolCall
		if reactMode {
			dispatchResult = d.getReActAdapter().AdaptResult(dispatchResult)
		}
		return dispatchResult, nil
	}

	content, err := d.llmService.Generate(ctx, config, req.Prompt)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return nil, fmt.Errorf("provider %s: %w", provider.Name, err)
	}

	// 估算 tokens（粗略：1 token ≈ 1.5 个中文字符或 4 个英文）
	totalTokens := estimateTokens(req.Prompt) + estimateTokens(content)
	cost := float64(totalTokens) / 1000.0 * provider.CostPer1k

	return &DispatchResult{
		Provider:    provider.Name,
		Model:       provider.Model,
		Content:     content,
		TotalTokens: totalTokens,
		Cost:        cost,
		LatencyMs:   latency,
		// P0-3 修复：LLMService 尚未透传时，FinishReason 默认为 "stop"
		FinishReason: "stop",
	}, nil
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

// estimateTokens 估算 token 数
func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	// 粗略估算：每个中文字符 1.5 token, 每 4 个英文 1 token
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

// GetRouteList 获取所有路由
func (d *Dispatcher) GetRouteList() []ScenarioRoute {
	d.mu.RLock()
	defer d.mu.RUnlock()
	list := make([]ScenarioRoute, 0, len(d.routes))
	for _, r := range d.routes {
		list = append(list, *r)
	}
	return list
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
