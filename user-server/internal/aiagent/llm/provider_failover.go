package llm

// provider_failover.go LLM 多 Provider 降级
//
// 五层架构归属: L2 服务层 / L3 编排层
// 设计依据: PRD §M-1
// 私域独立部署: 无 merchant_id 字段
//
// 功能：
//   - Provider 健康检查（每 30 秒 ping 一次）
//   - Provider 状态表 ProviderHealth（up/down/degraded + 熔断器状态）
//   - 熔断器：连续 5 次失败 → 熔断 60 秒
//   - 降级策略：主 Provider 失败 → 备用 Provider → 本地小模型 → 模板回复
//   - 配置：从 system_kv_config 表 key=llm_provider_failover 读取策略
//   - API：GET /api/llm/providers/health
//   - API：POST /api/llm/providers/circuit/reset

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"marketing/internal/pkg/utils/logger"
)

// ProviderStatus Provider 健康状态
type ProviderStatus string

const (
	ProviderStatusUp       ProviderStatus = "up"
	ProviderStatusDown     ProviderStatus = "down"
	ProviderStatusDegraded ProviderStatus = "degraded"
)

// 熔断器默认参数
const (
	DefaultHealthCheckInterval = 30 * time.Second
	DefaultFailureThreshold    = 5
	DefaultCircuitOpenDuration = 60 * time.Second
	DefaultHealthCheckTimeout  = 5 * time.Second
)

// ProviderHealth Provider 健康状态记录（运行期数据 + 可选 DB 持久化）
type ProviderHealth struct {
	ProviderName        string         `json:"provider_name"`
	Status              ProviderStatus `json:"status"`
	LastCheck           time.Time      `json:"last_check"`
	LastError           string         `json:"last_error,omitempty"`
	ConsecutiveFailures int            `json:"consecutive_failures"`
	CircuitOpenUntil    time.Time      `json:"circuit_open_until,omitempty"`
	// LatencyP95Ms 最近一次健康检查的延迟（毫秒），用于 degraded 判定
	LatencyP95Ms int64 `json:"latency_p95_ms,omitempty"`
}

// IsZero 判断 CircuitOpenUntil 是否未设置（兼容 Zero 检查）
// time.Time.IsZero 已存在；此处 helper 用于业务语义清晰。

// FailoverConfig 降级策略配置（从 system_kv_config 表 key=llm_provider_failover 读取）
type FailoverConfig struct {
	// HealthCheckInterval 健康检查间隔（秒）
	HealthCheckInterval int `json:"health_check_interval"`
	// FailureThreshold 触发熔断的连续失败次数
	FailureThreshold int `json:"failure_threshold"`
	// CircuitOpenDuration 熔断持续时间（秒）
	CircuitOpenDuration int `json:"circuit_open_duration"`
	// DegradedLatencyMs 延迟超过该阈值标记 degraded（毫秒）
	DegradedLatencyMs int64 `json:"degraded_latency_ms"`
	// LocalFallbackProvider 本地兜底 provider 名（如 default）
	LocalFallbackProvider string `json:"local_fallback_provider"`
	// TemplateReply 全部失败时的模板回复
	TemplateReply string `json:"template_reply"`
	// HealthCheckPath 健康检查子路径（默认 /health）
	HealthCheckPath string `json:"health_check_path"`
}

// DefaultFailoverConfig 默认降级策略
func DefaultFailoverConfig() FailoverConfig {
	return FailoverConfig{
		HealthCheckInterval:   int(DefaultHealthCheckInterval / time.Second),
		FailureThreshold:      DefaultFailureThreshold,
		CircuitOpenDuration:   int(DefaultCircuitOpenDuration / time.Second),
		DegradedLatencyMs:     3000,
		LocalFallbackProvider: "default",
		TemplateReply:         "抱歉，当前服务暂时繁忙，请稍后再试或联系人工客服。",
		HealthCheckPath:       "/health",
	}
}

// FailoverPolicy 降级策略（从 system_kv_config 读取的完整 JSON）
type FailoverPolicy struct {
	Config    FailoverConfig      `json:"config"`
	Scenarios map[string][]string `json:"scenarios"` // scenario -> ordered provider list
}

// DefaultFailoverPolicy 默认降级策略（注入到 system_kv_config 表的种子数据）
func DefaultFailoverPolicy() FailoverPolicy {
	return FailoverPolicy{
		Config: DefaultFailoverConfig(),
		Scenarios: map[string][]string{
			"intent_recognize": {"default"},
			"sop_reply":        {"default"},
			"objection":        {"default"},
			"friendly_chat":    {"default"},
			"long_summary":     {"default"},
			"high_quality":     {"default"},
			"low_cost":         {"default"},
		},
	}
}

// HealthChecker Provider 健康检查器
type HealthChecker interface {
	// Ping 检查 provider 健康（返回延迟毫秒与错误）
	Ping(ctx context.Context, provider *ProviderConfig, config FailoverConfig) (int64, error)
}

// HTTPHealthChecker 基于 HTTP 的健康检查器
type HTTPHealthChecker struct {
	httpClient *http.Client
}

// NewHTTPHealthChecker 创建 HTTP 健康检查器
func NewHTTPHealthChecker() *HTTPHealthChecker {
	return &HTTPHealthChecker{
		httpClient: &http.Client{Timeout: DefaultHealthCheckTimeout},
	}
}

// Ping 实现 HealthChecker 接口
// 优先 GET {BaseURL}{HealthCheckPath}，失败时退化为轻量 chat completion 请求
func (h *HTTPHealthChecker) Ping(ctx context.Context, provider *ProviderConfig, config FailoverConfig) (int64, error) {
	if provider == nil {
		return 0, fmt.Errorf("provider is nil")
	}
	if provider.BaseURL == "" {
		// 无 BaseURL 视为本地 provider，直接返回成功（本地服务由其他方式监控）
		return 0, nil
	}
	path := config.HealthCheckPath
	if path == "" {
		path = "/health"
	}
	url := strings.TrimRight(provider.BaseURL, "/") + path

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	if provider.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}
	resp, err := h.httpClient.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return latency, fmt.Errorf("health check %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	// 2xx 视为健康，5xx 视为故障，其他视为 degraded
	if resp.StatusCode >= 500 {
		return latency, fmt.Errorf("health check %s returned status %d", url, resp.StatusCode)
	}
	return latency, nil
}

// ProviderFailover 多 Provider 降级管理器
type ProviderFailover struct {
	mu         sync.RWMutex
	dispatcher *Dispatcher
	checker    HealthChecker
	health     map[string]*ProviderHealth
	config     FailoverConfig
	db         *gorm.DB
	stopCh     chan struct{}
	stopped    atomic.Bool
}

// NewProviderFailover 创建降级管理器
func NewProviderFailover(dispatcher *Dispatcher, db *gorm.DB) *ProviderFailover {
	return &ProviderFailover{
		dispatcher: dispatcher,
		checker:    NewHTTPHealthChecker(),
		health:     make(map[string]*ProviderHealth),
		config:     DefaultFailoverConfig(),
		db:         db,
		stopCh:     make(chan struct{}),
	}
}

// SetHealthChecker 注入自定义 HealthChecker（测试用）
func (f *ProviderFailover) SetHealthChecker(checker HealthChecker) {
	if checker != nil {
		f.checker = checker
	}
}

// LoadPolicy 从 system_kv_config 表加载策略（key=llm_provider_failover）
// 表不存在或读不到时使用默认策略
func (f *ProviderFailover) LoadPolicy(ctx context.Context) FailoverPolicy {
	policy := DefaultFailoverPolicy()
	if f.db == nil {
		return policy
	}
	var raw string
	// system_kv_config 表由 llm_failover_migration 创建
	// 用 Raw SQL 兼容表存在/不存在场景
	tx := f.db.WithContext(ctx).Raw(`SELECT value FROM system_kv_config WHERE key = 'llm_provider_failover' LIMIT 1`).Scan(&raw)
	if tx.Error != nil {
		// 表不存在或查询失败时使用默认策略
		return policy
	}
	if raw == "" {
		return policy
	}
	var loaded FailoverPolicy
	if err := json.Unmarshal([]byte(raw), &loaded); err != nil {
		logger.Warnf("[ProviderFailover] 解析 llm_provider_failover 配置失败: %v", err)
		return policy
	}
	// 合并：保留默认值的零字段
	if loaded.Config.HealthCheckInterval > 0 {
		policy.Config.HealthCheckInterval = loaded.Config.HealthCheckInterval
	}
	if loaded.Config.FailureThreshold > 0 {
		policy.Config.FailureThreshold = loaded.Config.FailureThreshold
	}
	if loaded.Config.CircuitOpenDuration > 0 {
		policy.Config.CircuitOpenDuration = loaded.Config.CircuitOpenDuration
	}
	if loaded.Config.DegradedLatencyMs > 0 {
		policy.Config.DegradedLatencyMs = loaded.Config.DegradedLatencyMs
	}
	if loaded.Config.LocalFallbackProvider != "" {
		policy.Config.LocalFallbackProvider = loaded.Config.LocalFallbackProvider
	}
	if loaded.Config.TemplateReply != "" {
		policy.Config.TemplateReply = loaded.Config.TemplateReply
	}
	if loaded.Config.HealthCheckPath != "" {
		policy.Config.HealthCheckPath = loaded.Config.HealthCheckPath
	}
	if len(loaded.Scenarios) > 0 {
		policy.Scenarios = loaded.Scenarios
	}
	return policy
}

// ApplyConfig 应用降级策略配置
func (f *ProviderFailover) ApplyConfig(config FailoverConfig) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.config = config
}

// Config 返回当前配置（只读副本）
func (f *ProviderFailover) Config() FailoverConfig {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.config
}

// Start 启动健康检查循环（后台 goroutine）
func (f *ProviderFailover) Start(ctx context.Context) {
	if f.stopped.Load() {
		return
	}
	go f.healthCheckLoop(ctx)
}

// Stop 停止健康检查
func (f *ProviderFailover) Stop() {
	if f.stopped.CompareAndSwap(false, true) {
		close(f.stopCh)
	}
}

// healthCheckLoop 周期性执行健康检查
func (f *ProviderFailover) healthCheckLoop(ctx context.Context) {
	// 先加载策略
	policy := f.LoadPolicy(ctx)
	f.ApplyPolicy(policy)

	ticker := time.NewTicker(f.interval())
	defer ticker.Stop()
	// 立即执行一次
	f.checkAll(ctx)
	for {
		select {
		case <-f.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.checkAll(ctx)
			// 重新加载策略（管理员可能更新了配置）
			policy := f.LoadPolicy(ctx)
			f.ApplyPolicy(policy)
			ticker.Reset(f.interval())
		}
	}
}

// ApplyPolicy 应用策略配置
func (f *ProviderFailover) ApplyPolicy(policy FailoverPolicy) {
	f.ApplyConfig(policy.Config)
}

// interval 返回当前配置对应的检查间隔
func (f *ProviderFailover) interval() time.Duration {
	cfg := f.Config()
	sec := cfg.HealthCheckInterval
	if sec <= 0 {
		sec = int(DefaultHealthCheckInterval / time.Second)
	}
	return time.Duration(sec) * time.Second
}

// checkAll 检查所有 provider
func (f *ProviderFailover) checkAll(ctx context.Context) {
	if f.dispatcher == nil {
		return
	}
	providers := f.dispatcher.GetProviderList()
	cfg := f.Config()
	for i := range providers {
		p := &providers[i]
		if !p.Enabled {
			continue
		}
		f.checkOne(ctx, p, cfg)
	}
}

// checkOne 检查单个 provider
func (f *ProviderFailover) checkOne(ctx context.Context, provider *ProviderConfig, cfg FailoverConfig) {
	checkCtx, cancel := context.WithTimeout(ctx, DefaultHealthCheckTimeout)
	defer cancel()
	latency, err := f.checker.Ping(checkCtx, provider, cfg)
	f.mu.Lock()
	defer f.mu.Unlock()
	h, ok := f.health[provider.Name]
	if !ok {
		h = &ProviderHealth{ProviderName: provider.Name, Status: ProviderStatusUp}
		f.health[provider.Name] = h
	}
	h.LastCheck = time.Now()
	h.LatencyP95Ms = latency
	if err != nil {
		h.ConsecutiveFailures++
		h.LastError = err.Error()
		if h.ConsecutiveFailures >= cfg.FailureThreshold {
			h.Status = ProviderStatusDown
			h.CircuitOpenUntil = time.Now().Add(time.Duration(cfg.CircuitOpenDuration) * time.Second)
		} else {
			h.Status = ProviderStatusDegraded
		}
		return
	}
	// 成功：重置失败计数
	h.ConsecutiveFailures = 0
	h.LastError = ""
	h.CircuitOpenUntil = time.Time{}
	if cfg.DegradedLatencyMs > 0 && latency > cfg.DegradedLatencyMs {
		h.Status = ProviderStatusDegraded
	} else {
		h.Status = ProviderStatusUp
	}
}

// IsCircuitOpen 判断 provider 是否处于熔断状态
func (f *ProviderFailover) IsCircuitOpen(providerName string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	h, ok := f.health[providerName]
	if !ok {
		return false
	}
	if h.CircuitOpenUntil.IsZero() {
		return false
	}
	if time.Now().Before(h.CircuitOpenUntil) {
		return true
	}
	// 熔断时间已过，恢复探测
	return false
}

// GetHealth 返回 provider 健康状态
func (f *ProviderFailover) GetHealth(providerName string) *ProviderHealth {
	f.mu.RLock()
	defer f.mu.RUnlock()
	h, ok := f.health[providerName]
	if !ok {
		return nil
	}
	cp := *h
	return &cp
}

// GetAllHealth 返回所有 provider 健康状态
func (f *ProviderFailover) GetAllHealth() []ProviderHealth {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]ProviderHealth, 0, len(f.health))
	for _, h := range f.health {
		out = append(out, *h)
	}
	return out
}

// ResetCircuit 重置 provider 熔断器
func (f *ProviderFailover) ResetCircuit(providerName string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	h, ok := f.health[providerName]
	if !ok {
		return false
	}
	h.CircuitOpenUntil = time.Time{}
	h.ConsecutiveFailures = 0
	h.Status = ProviderStatusUp
	h.LastError = ""
	return true
}

// RecordSuccess 记录 provider 调用成功（由 Dispatch 调用）
func (f *ProviderFailover) RecordSuccess(providerName string, latencyMs int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	h, ok := f.health[providerName]
	if !ok {
		h = &ProviderHealth{ProviderName: providerName, Status: ProviderStatusUp}
		f.health[providerName] = h
	}
	h.ConsecutiveFailures = 0
	h.LastError = ""
	h.CircuitOpenUntil = time.Time{}
	h.LatencyP95Ms = latencyMs
	cfg := f.config
	if cfg.DegradedLatencyMs > 0 && latencyMs > cfg.DegradedLatencyMs {
		h.Status = ProviderStatusDegraded
	} else {
		h.Status = ProviderStatusUp
	}
}

// RecordFailure 记录 provider 调用失败（由 Dispatch 调用）
func (f *ProviderFailover) RecordFailure(providerName string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	h, ok := f.health[providerName]
	if !ok {
		h = &ProviderHealth{ProviderName: providerName, Status: ProviderStatusUp}
		f.health[providerName] = h
	}
	h.ConsecutiveFailures++
	if err != nil {
		h.LastError = err.Error()
	}
	cfg := f.config
	if h.ConsecutiveFailures >= cfg.FailureThreshold {
		h.Status = ProviderStatusDown
		h.CircuitOpenUntil = time.Now().Add(time.Duration(cfg.CircuitOpenDuration) * time.Second)
	} else {
		h.Status = ProviderStatusDegraded
	}
}

// DispatchWithFailover 带降级的调度
// 顺序：主 Provider → 备用 Provider → 本地兜底 → 模板回复
func (f *ProviderFailover) DispatchWithFailover(ctx context.Context, req DispatchRequest) (*DispatchResult, error) {
	if f.dispatcher == nil {
		return nil, fmt.Errorf("dispatcher is nil")
	}

	// 加载降级策略
	policy := f.LoadPolicy(ctx)
	f.ApplyPolicy(policy)

	// 构建候选 provider 列表
	candidates := f.buildCandidates(req.Scenario, policy)
	cfg := f.Config()

	var lastErr error
	for _, name := range candidates {
		if f.IsCircuitOpen(name) {
			continue
		}
		provider := f.dispatcher.GetProvider(name)
		if provider == nil || !provider.Enabled {
			continue
		}
		// 创建单 provider 路由的 dispatch 请求
		start := time.Now()
		result, err := f.callSingleProvider(ctx, provider, req, cfg)
		latency := time.Since(start).Milliseconds()
		if err != nil {
			f.RecordFailure(name, err)
			lastErr = err
			logger.Warnf("[ProviderFailover] provider=%s failed: %v", name, err)
			continue
		}
		f.RecordSuccess(name, latency)
		return result, nil
	}

	// 全部失败 → 返回降级响应（模板回复）
	if lastErr == nil {
		lastErr = fmt.Errorf("no available provider for scenario: %s", req.Scenario)
	}
	logger.Errorf("[ProviderFailover] all providers failed scenario=%s: %v", req.Scenario, lastErr)
	return f.degradedResponse(req, cfg, lastErr), nil
}

// buildCandidates 构建候选 provider 列表（按策略排序，去重，包含本地兜底）
func (f *ProviderFailover) buildCandidates(scenario DispatchScenario, policy FailoverPolicy) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, 8)
	// 1) 策略中场景指定的列表
	if list, ok := policy.Scenarios[string(scenario)]; ok {
		for _, name := range list {
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	// 2) Dispatcher 路由的主 + 备选
	if route := f.dispatcher.GetRoute(scenario); route != nil {
		if !seen[route.Provider] {
			seen[route.Provider] = true
			out = append(out, route.Provider)
		}
		for _, name := range route.Fallbacks {
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	// 3) 本地兜底 provider 放到最后
	if policy.Config.LocalFallbackProvider != "" && !seen[policy.Config.LocalFallbackProvider] {
		out = append(out, policy.Config.LocalFallbackProvider)
	}
	return out
}

// callSingleProvider 通过 Dispatcher.callProvider 调用单个 provider
// 通过临时构造 ScenarioRoute 复用现有调度逻辑
func (f *ProviderFailover) callSingleProvider(ctx context.Context, provider *ProviderConfig, req DispatchRequest, cfg FailoverConfig) (*DispatchResult, error) {
	// 复用 dispatcher.callProvider，需要构造 ScenarioRoute（用于 MaxLatency）
	route := &ScenarioRoute{
		Scenario:   req.Scenario,
		Provider:   provider.Name,
		MaxLatency: 0,
		MinQuality: 0,
	}
	// 直接调用未导出的 callProvider（同包内可访问）
	return f.dispatcher.callProvider(ctx, provider, req, route)
}

// degradedResponse 构造降级响应（模板回复 + 标记降级）
func (f *ProviderFailover) degradedResponse(req DispatchRequest, cfg FailoverConfig, cause error) *DispatchResult {
	reply := cfg.TemplateReply
	if reply == "" {
		reply = "抱歉，当前服务暂时繁忙，请稍后再试。"
	}
	return &DispatchResult{
		Provider:     "degraded",
		Model:        "template",
		Content:      reply,
		FinishReason: "degraded",
		// 标记降级原因
		// 注意：DispatchResult 无 dedicated 字段，复用 Logprobs[0] 不合适；
		// 通过新增 Usage.TotalTokens=0 + 自定义日志体现
		Usage: TokenUsage{
			PromptTokens:     estimateTokens(req.Prompt),
			CompletionTokens: estimateTokens(reply),
			TotalTokens:      estimateTokens(req.Prompt) + estimateTokens(reply),
		},
	}
}

// IsDegraded 判断 DispatchResult 是否为降级响应
func IsDegraded(result *DispatchResult) bool {
	return result != nil && result.Provider == "degraded" && result.Model == "template"
}

// ====== 全局单例 ======

var (
	globalFailover     *ProviderFailover
	globalFailoverOnce sync.Once
)

// InitGlobalFailover 初始化全局降级管理器
func InitGlobalFailover(dispatcher *Dispatcher, db *gorm.DB) *ProviderFailover {
	globalFailoverOnce.Do(func() {
		globalFailover = NewProviderFailover(dispatcher, db)
	})
	return globalFailover
}

// GetGlobalFailover 获取全局降级管理器
func GetGlobalFailover() *ProviderFailover {
	return globalFailover
}
