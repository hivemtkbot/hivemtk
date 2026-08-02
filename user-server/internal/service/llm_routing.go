package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"marketing/internal/aiagent/llm"
	"marketing/internal/pkg/utils/logger"
)

// ============================================================================
// LLM 路由服务（重构于）
// ----------------------------------------------------------------------------
// 核心能力：
//  1. scenario 维度：LLMModelStat 拆为「按 provider」+「按 (provider, scenario)」
//  2. 路由变更走 SetRouteWithAudit（自动落 llm_routing_audit 表）
//  3. TestModel 走独立路径：直接调 dispatcher.callProvider，不动 routes map
//  4. Usage 读 llm_routing_logs 聚合（按 scenario+provider），同时返回 in-memory 实时数据
//  5. CostStats 暴露按场景维度的详细统计
//  6. Stats 读 llm_routing_logs 跨进程历史
// ============================================================================

// LLMModelStat 内存中"按 provider"实时累计（用于 /stats 实时面板）
//
// 该 map 在 Dispatcher 决策日志落库后，仅保留"最近一次统计清点后的增量"。
// 真实历史数据走 llm_routing_logs 表，通过 Stats 端点聚合查询。
type LLMModelStat struct {
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	CallCount    int64     `json:"call_count"`
	SuccessCount int64     `json:"success_count"`
	FailedCount  int64     `json:"failed_count"`
	TotalTokens  int64     `json:"total_tokens"`
	TotalCost    float64   `json:"total_cost"`
	AvgLatencyMs int64     `json:"avg_latency_ms"` // 最近一次写入的延迟（不是真平均）
	LastUsedAt   time.Time `json:"last_used_at"`
}

// LLMProviderInfo 暴露给前端的 provider 信息
type LLMProviderInfo struct {
	Name         string   `json:"name"`
	DisplayName  string   `json:"display_name,omitempty"`
	BaseURL      string   `json:"base_url"`
	Model        string   `json:"model"`
	APIKey       string   `json:"api_key,omitempty"` // 创建/更新时设置；列表仅返回是否设置（APIKeySet）
	APIKeySet    bool     `json:"api_key_set"`
	Enabled      bool     `json:"enabled"`
	QualityScore float64  `json:"quality_score"`
	MaxRPM       int      `json:"max_rpm"`
	CostPer1k    float64  `json:"cost_per_1k"`
	AvgLatencyMs int      `json:"avg_latency_ms"`
	NoFC         bool     `json:"no_fc"`
	Vendor       string   `json:"vendor"`
	Tags         []string `json:"tags,omitempty"`
}

// UpdateStrategiesRequest 批量更新场景路由请求
type UpdateStrategiesRequest struct {
	Routes    []llm.ScenarioRoute `json:"routes"`
	Operator  string              `json:"operator"`
	TraceID   string              `json:"trace_id"`
	CommitMsg string              `json:"commit_msg"`
}

// UsageSummary 用量汇总（按场景维度）
type UsageSummary struct {
	TotalCalls     int64              `json:"total_calls"`
	TotalSuccess   int64              `json:"total_success"`
	TotalFailed    int64              `json:"total_failed"`
	TotalTokens    int64              `json:"total_tokens"`
	TotalCost      float64            `json:"total_cost"`
	ActiveModels   int64              `json:"active_models"`
	EnabledModels  int64              `json:"enabled_models"`
	WindowLabel    string             `json:"window_label"`
	ByScenario     []ScenarioUsage    `json:"by_scenario"`
	ByProvider     []ProviderUsage    `json:"by_provider"`
	ByScenarioProv []llm.ScenarioStat `json:"by_scenario_provider"`
}

// ScenarioUsage 场景维度用量
type ScenarioUsage struct {
	Scenario    string  `json:"scenario"`
	CallCount   int64   `json:"call_count"`
	TotalTokens int64   `json:"total_tokens"`
	TotalCost   float64 `json:"total_cost"`
}

// ProviderUsage provider 维度用量
type ProviderUsage struct {
	Provider    string  `json:"provider"`
	CallCount   int64   `json:"call_count"`
	TotalTokens int64   `json:"total_tokens"`
	TotalCost   float64 `json:"total_cost"`
}

// TestModelRequest 测试模型请求
type TestModelRequest struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Prompt   string `json:"prompt"`
	Timeout  int    `json:"timeout_seconds"`
}

// TestModelResult 测试模型结果
type TestModelResult struct {
	Provider    string  `json:"provider"`
	Model       string  `json:"model"`
	Content     string  `json:"content"`
	LatencyMs   int64   `json:"latency_ms"`
	TotalTokens int     `json:"total_tokens"`
	Cost        float64 `json:"cost"`
	Success     bool    `json:"success"`
	Error       string  `json:"error,omitempty"`
}

// LLMRoutingService LLM 路由服务
type LLMRoutingService struct {
	dispatcher *llm.Dispatcher

	mu    sync.Mutex
	stats map[string]*LLMModelStat // key=provider，进程内实时累计
}

// NewLLMRoutingService 创建 LLM 路由服务
func NewLLMRoutingService(d *llm.Dispatcher) *LLMRoutingService {
	return &LLMRoutingService{
		dispatcher: d,
		stats:      make(map[string]*LLMModelStat),
	}
}

// ============================================================================
// 模型管理
// ============================================================================

// ListModels 列出所有 provider
func (s *LLMRoutingService) ListModels(ctx context.Context) []LLMProviderInfo {
	if s.dispatcher == nil {
		return nil
	}
	providers := s.dispatcher.GetAllProviders()
	out := make([]LLMProviderInfo, 0, len(providers))
	for _, p := range providers {
		out = append(out, LLMProviderInfo{
			Name:         p.Name,
			DisplayName:  p.DisplayName,
			BaseURL:      p.BaseURL,
			Model:        p.Model,
			APIKeySet:    p.APIKey != "",
			Enabled:      p.Enabled,
			QualityScore: p.QualityScore,
			MaxRPM:       p.MaxRPM,
			CostPer1k:    p.CostPer1k,
			AvgLatencyMs: p.AvgLatencyMs,
			NoFC:         p.NoFC,
			Vendor:       vendorOf(p),
			Tags:         p.Tags,
		})
	}
	return out
}

// ResolveProviderName 将路径参数（可能是 provider Name 或 Model）解析为实际的 provider Name。
// 优先按 Name 精确匹配，其次按 Model 精确匹配。未找到返回空字符串。
// 这支撑前端 /api/llm/models/:name 端点同时接受 provider name（如 "deepseek"）和 model name（如 "gpt-4o"）。
func (s *LLMRoutingService) ResolveProviderName(name string) string {
	if s.dispatcher == nil || name == "" {
		return ""
	}
	// 1. 按 provider Name 精确匹配
	if s.dispatcher.GetProvider(name) != nil {
		return name
	}
	// 2. 按 Model 字段精确匹配
	for _, p := range s.dispatcher.GetAllProviders() {
		if p.Model == name {
			return p.Name
		}
	}
	return ""
}

// CreateModel 注册新 provider（内存 + 落库 llm_providers，重启不丢）
func (s *LLMRoutingService) CreateModel(ctx context.Context, info LLMProviderInfo) error {
	if s.dispatcher == nil {
		return errors.New("dispatcher not initialized")
	}
	if info.Name == "" {
		return errors.New("name is required")
	}
	if info.BaseURL == "" {
		return errors.New("base_url is required")
	}
	if info.Model == "" {
		return errors.New("model is required")
	}
	if info.QualityScore <= 0 {
		info.QualityScore = 0.8
	}
	if info.MaxRPM <= 0 {
		info.MaxRPM = 60
	}
	pc := llm.ProviderConfig{
		Name:         info.Name,
		DisplayName:  info.DisplayName,
		BaseURL:      info.BaseURL,
		Model:        info.Model,
		APIKey:       info.APIKey,
		APIType:      "openai",
		Enabled:      info.Enabled,
		QualityScore: info.QualityScore,
		MaxRPM:       info.MaxRPM,
		CostPer1k:    info.CostPer1k,
		NoFC:         info.NoFC,
		Vendor:       info.Vendor,
		Tags:         info.Tags,
	}
	s.dispatcher.AddProvider(pc) // 内存立即生效
	// 落库：容器重启后经 LoadProvidersFromDB 重新加载，避免丢失
	if err := s.dispatcher.UpsertProviderToDB(pc); err != nil {
		return fmt.Errorf("provider 落库失败: %w", err)
	}
	// 审计：注册新模型（不污染 routes map，直接写 audit）
	s.dispatcher.LogModelLifecycle(ctx,
		"create_model", info.Name, operatorFromContext(ctx), logger.TraceIDFromContext(ctx))
	return nil
}

// APIKeyClearSentinel API Key 清空标记
//
// 前端表单"留空表示不修改"语义：传 "" 保留原 key。
// 如需真正清空（删除 provider 的 API Key），前端传此特殊字符串。
const APIKeyClearSentinel = "__CLEAR_API_KEY__"

// UpdateModel 更新 provider
//
// 注意：identifier 语义为 provider name（与前端约定保持一致）。
// 返回旧 provider 信息供前端做 before/after 对比。
// 字段语义：
//   - BaseURL/Model: 空字符串表示保留原值
//   - APIKey:        空字符串保留原值；APIKeyClearSentinel 表示清空；其他覆盖
//   - 其他字段:      按需更新
func (s *LLMRoutingService) UpdateModel(ctx context.Context, identifier string, info LLMProviderInfo) error {
	if s.dispatcher == nil {
		return errors.New("dispatcher not initialized")
	}
	if identifier == "" {
		return errors.New("name is required")
	}
	old := s.dispatcher.GetProvider(identifier)
	if old == nil {
		return fmt.Errorf("provider %q not found", identifier)
	}
	// API Key 处理：空 = 保留；清空标记 = 清空；其他 = 覆盖
	apiKey := old.APIKey
	switch {
	case info.APIKey == APIKeyClearSentinel:
		apiKey = ""
	case info.APIKey != "":
		apiKey = info.APIKey
	}
	updated := llm.ProviderConfig{
		Name:         identifier, // 不允许重命名（避免 routes 引用断裂）
		DisplayName:  orDefault(info.DisplayName, old.DisplayName),
		BaseURL:      orDefault(info.BaseURL, old.BaseURL),
		Model:        orDefault(info.Model, old.Model),
		APIKey:       apiKey,
		APIType:      old.APIType, // LLMProviderInfo 无此字段，沿用原值
		Enabled:      info.Enabled,
		QualityScore: nonZero(info.QualityScore, old.QualityScore),
		MaxRPM:       nonZeroInt(info.MaxRPM, old.MaxRPM),
		CostPer1k:    info.CostPer1k,
		NoFC:         info.NoFC,
		Vendor:       orDefault(info.Vendor, old.Vendor),
		Tags:         info.Tags,
	}
	s.dispatcher.AddProvider(updated) // 内存立即生效
	// 落库：镜像内存态（含 APIKey：空=保留旧值由上面 apiKey 解析决定，清空标记=清空）
	if err := s.dispatcher.UpsertProviderToDB(updated); err != nil {
		return fmt.Errorf("provider 落库失败: %w", err)
	}
	return nil
}

// DeleteModel 删除 provider
//
// 行为：
//   - 注销 provider
//   - 清理内存 stats
//   - 审计：记录删除动作
func (s *LLMRoutingService) DeleteModel(ctx context.Context, identifier string) error {
	if s.dispatcher == nil {
		return errors.New("dispatcher not initialized")
	}
	if identifier == "" {
		return errors.New("name is required")
	}
	if identifier == "default" {
		return errors.New("本地默认模型 default 不可删除")
	}
	if s.dispatcher.GetProvider(identifier) == nil {
		return fmt.Errorf("provider %q not found", identifier)
	}
	if !s.dispatcher.RemoveProvider(identifier) {
		return fmt.Errorf("provider %q remove failed", identifier)
	}
	// 落库删除（default 之外均从 llm_providers 移除，重启不再加载）
	if err := s.dispatcher.DeleteProviderFromDB(identifier); err != nil {
		return fmt.Errorf("provider 落库删除失败: %w", err)
	}
	// 清理内存 stats
	s.mu.Lock()
	delete(s.stats, identifier)
	s.mu.Unlock()
	// 审计
	s.dispatcher.LogModelLifecycle(ctx,
		"delete_model", identifier, operatorFromContext(ctx), logger.TraceIDFromContext(ctx))
	return nil
}

// ============================================================================
// 场景路由管理
// ============================================================================

// ListStrategies 列出所有场景路由
func (s *LLMRoutingService) ListStrategies(ctx context.Context) []llm.ScenarioRoute {
	if s.dispatcher == nil {
		return nil
	}
	return s.dispatcher.GetAllRoutes()
}

// UpdateStrategies 批量更新场景路由
//
// 走 SetRouteWithAudit：自动版本号自增 + 审计日志。
// 兼容前端 List.vue 4 种调用形式（routes 数组 / 单个 route）。
func (s *LLMRoutingService) UpdateStrategies(ctx context.Context, req UpdateStrategiesRequest) error {
	if s.dispatcher == nil {
		return errors.New("dispatcher not initialized")
	}
	operator := orDefault(req.Operator, operatorFromContext(ctx))
	traceID := orDefault(req.TraceID, logger.TraceIDFromContext(ctx))
	if len(req.Routes) == 0 {
		return errors.New("routes is required")
	}
	for _, r := range req.Routes {
		if r.Scenario == "" {
			return errors.New("scenario is required")
		}
		if r.Provider == "" {
			return fmt.Errorf("scenario %q: provider is required", r.Scenario)
		}
		// canary 子路由也校验 provider 非空
		if r.Weight > 0 && r.Weight < 100 && r.CanaryRoute != nil {
			if r.CanaryRoute.Provider == "" {
				return fmt.Errorf("scenario %q: canary route provider is required", r.Scenario)
			}
		}
		s.dispatcher.SetRouteWithAudit(ctx, r, "update_strategy", operator, traceID)
	}
	return nil
}

// ListAuditHistory 查路由变更审计历史
func (s *LLMRoutingService) ListAuditHistory(ctx context.Context, scenario string, limit int) ([]map[string]any, error) {
	return llm.QueryAuditHistory(ctx, scenario, limit)
}

// ============================================================================
// 用量统计
// ============================================================================

// Stats 返回进程内实时 provider 维度统计
func (s *LLMRoutingService) Stats(ctx context.Context) map[string]LLMModelStat {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]LLMModelStat, len(s.stats))
	for k, v := range s.stats {
		out[k] = *v
	}
	return out
}

// Usage 跨进程历史统计（按 window 维度走 llm_routing_logs 聚合）
//
// window: today / week / month / all
func (s *LLMRoutingService) Usage(ctx context.Context, window string) (*UsageSummary, error) {
	if window == "" {
		window = "all"
	}
	enabledProviders := llm.GetGlobalDispatcher().GetEnabledProviderNames()
	stats, err := llm.QueryScenarioStats(ctx, window, 200, enabledProviders)
	if err != nil {
		return nil, err
	}
	summary := &UsageSummary{
		WindowLabel:    window,
		ByScenarioProv: stats,
	}
	scenarioMap := make(map[string]*ScenarioUsage)
	providerMap := make(map[string]*ProviderUsage)
	providerEnabled := make(map[string]bool)
	for _, st := range stats {
		summary.TotalCalls += st.CallCount
		summary.TotalSuccess += st.SuccessCount
		summary.TotalFailed += st.FailedCount
		summary.TotalTokens += st.TotalTokens
		summary.TotalCost += st.TotalCost

		if _, ok := scenarioMap[st.Scenario]; !ok {
			scenarioMap[st.Scenario] = &ScenarioUsage{Scenario: st.Scenario}
		}
		sc := scenarioMap[st.Scenario]
		sc.CallCount += st.CallCount
		sc.TotalTokens += st.TotalTokens
		sc.TotalCost += st.TotalCost

		if _, ok := providerMap[st.Provider]; !ok {
			providerMap[st.Provider] = &ProviderUsage{Provider: st.Provider}
		}
		pr := providerMap[st.Provider]
		pr.CallCount += st.CallCount
		pr.TotalTokens += st.TotalTokens
		pr.TotalCost += st.TotalCost
		providerEnabled[st.Provider] = true
	}
	for _, sc := range scenarioMap {
		summary.ByScenario = append(summary.ByScenario, *sc)
	}
	for _, pr := range providerMap {
		summary.ByProvider = append(summary.ByProvider, *pr)
	}
	// enabled/active 模型数从 dispatcher 实时取（更准）
	if s.dispatcher != nil {
		providers := s.dispatcher.GetAllProviders()
		for _, p := range providers {
			if p.Enabled {
				summary.EnabledModels++
			}
		}
		summary.ActiveModels = int64(len(providers))
	}
	return summary, nil
}

// ============================================================================
// 模型连通性测试
// ============================================================================

// TestModel 测试模型连通性（走独立路径，不污染全局路由/告警/熔断）
//
// 关键设计：
//   - 不动 routes map（避免破坏并发安全）
//   - 不调 dispatcher.Dispatch（避免触发告警、熔断、健康度累计）
//   - 直接通过 dispatcher.callProvider 走单次 provider 调用
//   - 用专属 timeout（默认 60s），避免被 route.MaxLatency 截断
//
// 返回：TestModelResult 含 Content/LatencyMs/Tokens/Cost/Success/Error
func (s *LLMRoutingService) TestModel(ctx context.Context, req TestModelRequest) (*TestModelResult, error) {
	if s.dispatcher == nil {
		return nil, errors.New("dispatcher not initialized")
	}
	if req.Provider == "" {
		return nil, errors.New("provider is required")
	}
	provider := s.dispatcher.GetProvider(req.Provider)
	if provider == nil {
		return nil, fmt.Errorf("provider %q not found", req.Provider)
	}
	if req.Prompt == "" {
		req.Prompt = "你好，请用一句话自我介绍。"
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 60
	}
	// 独立 ctx，规避 trace_id 污染
	tCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// 构造一个走测试 provider 的临时 route（不动 dispatcher 全局）
	testRoute := &llm.ScenarioRoute{
		Scenario:   llm.ScenarioLowCost,
		Provider:   req.Provider,
		MaxLatency: timeout * 1000, // 拉宽
		MinQuality: 0,
	}
	// 走 dispatcher 的私有 callProvider（不走 Dispatch，避免告警/熔断/统计）
	// 我们用反射私有方法不安全；改走临时构造 DispatchRequest + 直接 HTTP
	// 这里复用 GenerateWithTools 的服务端 LLMService
	dreq := llm.DispatchRequest{
		Scenario:    llm.ScenarioLowCost,
		Prompt:      req.Prompt,
		MaxTokens:   256,
		Temperature: 0.7,
		CanaryKey:   "_test_only_" + logger.TraceIDFromContext(ctx),
	}
	// 重要：直接走 dispatcher 公开方法，但用 isTest 路径
	result, err := s.dispatcher.CallProviderForTest(tCtx, provider, dreq, testRoute)
	res := &TestModelResult{
		Provider: req.Provider,
		Model:    provider.Model,
	}
	if err != nil {
		res.Success = false
		res.Error = err.Error()
		return res, nil
	}
	res.Content = result.Content
	res.LatencyMs = int64(result.LatencyMs)
	res.TotalTokens = result.Usage.TotalTokens
	res.Cost = result.Cost
	res.Success = true
	return res, nil
}

// ============================================================================
// 内部工具
// ============================================================================

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func nonZero(v, def float64) float64 {
	if v == 0 {
		return def
	}
	return v
}

func nonZeroInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

// vendorOf 返回 provider 厂商名：优先落库值，否则从 base_url 推断
func vendorOf(p llm.ProviderConfig) string {
	if p.Vendor != "" {
		return p.Vendor
	}
	return vendorFromBaseURL(p.BaseURL)
}

func vendorFromBaseURL(base string) string {
	low := strings.ToLower(base)
	switch {
	case strings.Contains(low, "deepseek"):
		return "deepseek"
	case strings.Contains(low, "dashscope"), strings.Contains(low, "qwen"):
		return "qwen"
	case strings.Contains(low, "openai"), strings.Contains(low, "gpt"):
		return "openai"
	case strings.Contains(low, "zhipu"), strings.Contains(low, "glm"):
		return "zhipu"
	case strings.Contains(low, "moonshot"), strings.Contains(low, "kimi"):
		return "moonshot"
	case strings.Contains(low, "127.0.0.1"), strings.Contains(low, "localhost"), strings.Contains(low, "mtk-llm"):
		return "local"
	default:
		return "other"
	}
}

func operatorFromContext(ctx context.Context) string {
	if v := ctx.Value("operator"); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	if v := ctx.Value("user_id"); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return "system"
}
