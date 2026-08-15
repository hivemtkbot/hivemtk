package service

import (
	"context"
	"errors"
	"time"

	"hivemtk-user/internal/aiagent/llm"
)


// LLMProviderHealth provider 健康状态（mirror llm.ProviderHealth）
type LLMProviderHealth struct {
	ProviderName        string    `json:"provider_name"`
	Status              string    `json:"status"`
	LastCheck           time.Time `json:"last_check"`
	LastError           string    `json:"last_error,omitempty"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	CircuitOpenUntil    time.Time `json:"circuit_open_until,omitempty"`
	LatencyP95Ms        int64     `json:"latency_p95_ms,omitempty"`
}

// LLMFailoverConfig 降级策略配置（mirror llm.FailoverConfig）
type LLMFailoverConfig struct {
	HealthCheckInterval   int    `json:"health_check_interval"`
	FailureThreshold      int    `json:"failure_threshold"`
	CircuitOpenDuration   int    `json:"circuit_open_duration"`
	DegradedLatencyMs     int64  `json:"degraded_latency_ms"`
	LocalFallbackProvider string `json:"local_fallback_provider"`
	TemplateReply         string `json:"template_reply"`
	HealthCheckPath       string `json:"health_check_path"`
}

// LLMFailoverPolicy 降级策略（mirror llm.FailoverPolicy）
type LLMFailoverPolicy struct {
	Config    LLMFailoverConfig   `json:"config"`
	Scenarios map[string][]string `json:"scenarios"`
}

// LLMScenarioRoute 场景路由（mirror llm.ScenarioRoute，用于兼容单 route 更新端点）
type LLMScenarioRoute struct {
	Scenario    string            `json:"scenario"`
	Provider    string            `json:"provider"`
	Fallbacks   []string          `json:"fallbacks"`
	CostWeight  int               `json:"cost_weight"`
	MaxLatency  int               `json:"max_latency"`
	MinQuality  float64           `json:"min_quality"`
	Version     int               `json:"version"`
	Weight      int               `json:"weight"`
	CanaryKey   string            `json:"canary_key,omitempty"`
	CanaryRoute *LLMScenarioRoute `json:"canary_route,omitempty"`
}

// LLMRouteResolution 场景路由决策结果（mirror dispatcher GetRoute + canary 判定输出）
type LLMRouteResolution struct {
	Provider  string   `json:"provider"`
	Fallbacks []string `json:"fallbacks"`
	Version   int      `json:"version"`
	Weight    int      `json:"weight"`
	IsCanary  bool     `json:"is_canary"`
}

// LLMFailoverService LLM Provider 降级管理服务（system 域）
type LLMFailoverService struct {
	failover *llm.ProviderFailover
}

// NewLLMFailoverService 创建降级管理服务；failover 可为 nil（端点返回 503 语义由 controller 守卫）
func NewLLMFailoverService(f *llm.ProviderFailover) *LLMFailoverService {
	return &LLMFailoverService{failover: f}
}

// Ready failover 是否已注入
func (s *LLMFailoverService) Ready() bool {
	return s != nil && s.failover != nil
}

// GetAllHealth 查询所有 provider 健康度
func (s *LLMFailoverService) GetAllHealth() []LLMProviderHealth {
	if !s.Ready() {
		return nil
	}
	hs := s.failover.GetAllHealth()
	out := make([]LLMProviderHealth, 0, len(hs))
	for i := range hs {
		out = append(out, fromLLMProviderHealth(hs[i]))
	}
	return out
}

// GetHealth 查询单个 provider 健康度，未找到返回 nil
func (s *LLMFailoverService) GetHealth(name string) *LLMProviderHealth {
	if !s.Ready() {
		return nil
	}
	h := s.failover.GetHealth(name)
	if h == nil {
		return nil
	}
	m := fromLLMProviderHealth(*h)
	return &m
}

// ResetCircuit 重置单个 provider 熔断器
func (s *LLMFailoverService) ResetCircuit(name string) bool {
	if !s.Ready() {
		return false
	}
	return s.failover.ResetCircuit(name)
}

// LoadPolicy 查询降级策略
func (s *LLMFailoverService) LoadPolicy(ctx context.Context) LLMFailoverPolicy {
	if !s.Ready() {
		return LLMFailoverPolicy{}
	}
	return fromLLMFailoverPolicy(s.failover.LoadPolicy(ctx))
}

// ApplyPolicy 更新降级策略（内存生效）
func (s *LLMFailoverService) ApplyPolicy(policy LLMFailoverPolicy) {
	if !s.Ready() {
		return
	}
	s.failover.ApplyPolicy(toLLMFailoverPolicy(policy))
}

// IsCircuitOpen 查询 provider 是否处于熔断状态
func (s *LLMFailoverService) IsCircuitOpen(name string) bool {
	if !s.Ready() {
		return false
	}
	return s.failover.IsCircuitOpen(name)
}

// ErrLLMDispatcherNotReady 全局 dispatcher 未初始化
var ErrLLMDispatcherNotReady = errors.New("dispatcher not initialized")

// ErrLLMScenarioRouteNotFound 场景路由不存在
var ErrLLMScenarioRouteNotFound = errors.New("scenario route not found")

// ResolveRoute 按 scenario + canary key 决策路由（走全局 dispatcher 单例）
func (s *LLMFailoverService) ResolveRoute(scenario, canaryKey string) (*LLMRouteResolution, error) {
	d := llm.GetGlobalDispatcher()
	if d == nil {
		return nil, ErrLLMDispatcherNotReady
	}
	route := d.GetRoute(llm.DispatchScenario(scenario))
	if route == nil {
		return nil, ErrLLMScenarioRouteNotFound
	}
	res := &LLMRouteResolution{
		Provider:  route.Provider,
		Fallbacks: route.Fallbacks,
		Version:   route.Version,
		Weight:    route.Weight,
	}
	if canary := llm.DecideCanaryRoute(route, canaryKey); canary != nil {
		res.Provider = canary.Provider
		res.Fallbacks = canary.Fallbacks
		res.Version = canary.Version
		res.IsCanary = true
	}
	return res, nil
}

// toLLMScenarioRoute mirror 转 llm.ScenarioRoute（含递归 canary route）
func toLLMScenarioRoute(r LLMScenarioRoute) llm.ScenarioRoute {
	out := llm.ScenarioRoute{
		Scenario:    llm.DispatchScenario(r.Scenario),
		Provider:    r.Provider,
		Fallbacks:   r.Fallbacks,
		CostWeight:  r.CostWeight,
		MaxLatency:  r.MaxLatency,
		MinQuality:  r.MinQuality,
		Version:     r.Version,
		Weight:      r.Weight,
		CanaryKey:   r.CanaryKey,
		CanaryRoute: nil,
	}
	if r.CanaryRoute != nil {
		c := toLLMScenarioRoute(*r.CanaryRoute)
		out.CanaryRoute = &c
	}
	return out
}

// UpdateSingleScenarioRoute 兼容旧接口的单 route 批量更新入口
func (s *LLMRoutingService) UpdateSingleScenarioRoute(ctx context.Context, route LLMScenarioRoute) error {
	batch := UpdateStrategiesRequest{Routes: []llm.ScenarioRoute{toLLMScenarioRoute(route)}}
	return s.UpdateStrategies(ctx, batch)
}


func fromLLMProviderHealth(h llm.ProviderHealth) LLMProviderHealth {
	return LLMProviderHealth{
		ProviderName:        h.ProviderName,
		Status:              string(h.Status),
		LastCheck:           h.LastCheck,
		LastError:           h.LastError,
		ConsecutiveFailures: h.ConsecutiveFailures,
		CircuitOpenUntil:    h.CircuitOpenUntil,
		LatencyP95Ms:        h.LatencyP95Ms,
	}
}

func fromLLMFailoverPolicy(p llm.FailoverPolicy) LLMFailoverPolicy {
	return LLMFailoverPolicy{
		Config: LLMFailoverConfig{
			HealthCheckInterval:   p.Config.HealthCheckInterval,
			FailureThreshold:      p.Config.FailureThreshold,
			CircuitOpenDuration:   p.Config.CircuitOpenDuration,
			DegradedLatencyMs:     p.Config.DegradedLatencyMs,
			LocalFallbackProvider: p.Config.LocalFallbackProvider,
			TemplateReply:         p.Config.TemplateReply,
			HealthCheckPath:       p.Config.HealthCheckPath,
		},
		Scenarios: p.Scenarios,
	}
}

func toLLMFailoverPolicy(p LLMFailoverPolicy) llm.FailoverPolicy {
	return llm.FailoverPolicy{
		Config: llm.FailoverConfig{
			HealthCheckInterval:   p.Config.HealthCheckInterval,
			FailureThreshold:      p.Config.FailureThreshold,
			CircuitOpenDuration:   p.Config.CircuitOpenDuration,
			DegradedLatencyMs:     p.Config.DegradedLatencyMs,
			LocalFallbackProvider: p.Config.LocalFallbackProvider,
			TemplateReply:         p.Config.TemplateReply,
			HealthCheckPath:       p.Config.HealthCheckPath,
		},
		Scenarios: p.Scenarios,
	}
}

