// 拆分自 dispatcher.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"hivemtk-user/internal/cache"
	"hivemtk-user/internal/pkg/utils/logger"
	"strings"
	"sync"
	"time"
)

func degradedReply(req DispatchRequest) *DispatchResult {
	tmpl := "抱歉，当前客服系统繁忙，请稍后再试，或联系人工客服获取帮助。"
	if fo := GetGlobalFailover(); fo != nil {
		if c := fo.Config().TemplateReply; c != "" {
			tmpl = c
		}
	}
	promptTokens := estimateTokens(req.Prompt)
	completionTokens := estimateTokens(tmpl)
	return &DispatchResult{
		Provider:     "degraded",
		Model:        "template",
		Content:      tmpl,
		FinishReason: "degraded",
		Usage: TokenUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}
}

// allowRequest RPM 限流（全局，跨实例一致）
// 业务需要：必须守住上游 provider 每分钟配额/成本。多实例下若各持独立计数，
// 全局实际允许量被放大为 N×单实例配额，可能击穿上游限流/资费。
// 实现：REDIS_HOST 配置时走全局缓存固定窗口计数（Redis 共享，各实例累计同一配额）；
// 未配置 Redis 时回退进程内计数（单实例安全、零额外开销）。后端异常一律放行（可用性优先）。
func (d *Dispatcher) allowRequest(providerName string, maxRPM int) bool {
	if maxRPM <= 0 {
		return true
	}
	if !cache.GlobalIsRedis() {
		return d.allowRequestLocal(providerName, maxRPM)
	}
	c := cache.GetGlobalCache()
	key := fmt.Sprintf("mtk:llm:rpm:%s:%d", providerName, time.Now().Truncate(time.Minute).Unix())
	cur, err := c.Incr(context.Background(), key, time.Minute)
	if err != nil {
		logger.Warnf("[LLM] RPM 计数后端异常，放行 provider=%s: %v", providerName, err)
		return true
	}
	return cur <= int64(maxRPM)
}

// allowRequestLocal 单实例 RPM 限流（未配置 Redis 时走进程内固定窗口计数）
func (d *Dispatcher) allowRequestLocal(providerName string, maxRPM int) bool {
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
		// 推理模型（reasoning）在 reasoning 阶段需占用较多 token，过小的上限会截断到空回复。
		// 基线 max_tokens≥2048（与 SalesEngine.runAgentLoop 默认对齐），避免非 Agent 路径被截断。
		maxTokens = 2048
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
		MaxRetries:     1, // 性能优化：2→1。本地 LLM 失败多为进程崩溃/OOM，重试无意义只会空等 backoff（2s+4s）。失败由 dispatcher 候选 provider failover 兜底。
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
		Usage:          usage,
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
		// 集群熔断：该 provider 已被熔断（跨实例共享信号）则跳过，避免无效投票
		if fo := GetGlobalFailover(); fo != nil && fo.IsCircuitOpen(name) {
			logger.Debugf("[LLM] DispatchMultiModel provider=%s 集群熔断中，跳过", name)
			continue
		}
		result, err := d.callProvider(ctx, provider, req, &ScenarioRoute{MaxLatency: 10000})
		if err != nil {
			// 真实失败记录到集群熔断器，可跨实例触发/传播熔断信号
			if fo := GetGlobalFailover(); fo != nil {
				fo.RecordFailure(name, err)
			}
			continue
		}
		// 成功：清零连续失败计数，并删除跨实例熔断信号
		if fo := GetGlobalFailover(); fo != nil {
			fo.RecordSuccess(name, int64(result.LatencyMs))
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
