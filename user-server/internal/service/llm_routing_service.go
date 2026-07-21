package service

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"marketing/internal/aiagent/llm"
)

// LLMRoutingService LLM 多模型路由管理服务
// 基于全局 llm.Dispatcher 单例管理厂商(Provider)与场景路由(ScenarioRoute)，
// 并在内存中累计调用统计。厂商与路由配置随 Dispatcher 实例存在，重启后恢复为默认值。
type LLMRoutingService struct {
	dispatcher *llm.Dispatcher
	mu         sync.Mutex
	stats      map[string]*LLMModelStat // key = provider name
}

// LLMModelStat 单个模型的调用统计
type LLMModelStat struct {
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	CallCount    int64   `json:"call_count"`
	SuccessCount int64   `json:"success_count"`
	FailedCount  int64   `json:"failed_count"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalCost    float64 `json:"total_cost"`
	AvgLatencyMs int64   `json:"avg_latency_ms"`
	LastUsedAt   int64   `json:"last_used_at"` // unix seconds, 0 表示从未使用
}

// NewLLMRoutingService 创建 LLM 路由管理服务
func NewLLMRoutingService() *LLMRoutingService {
	return &LLMRoutingService{
		dispatcher: llm.GetGlobalDispatcher(),
		stats:      make(map[string]*LLMModelStat),
	}
}

// ListModels 列出所有模型(厂商)配置，按优先级(quality_score 降序)排序
func (s *LLMRoutingService) ListModels() []llm.ProviderConfig {
	list := s.dispatcher.GetProviderList()
	sort.Slice(list, func(i, j int) bool {
		return list[i].QualityScore > list[j].QualityScore
	})
	return list
}

// GetModel 获取单个模型
func (s *LLMRoutingService) GetModel(name string) (*llm.ProviderConfig, error) {
	if name == "" {
		return nil, errors.New("模型名称不能为空")
	}
	p := s.dispatcher.GetProvider(name)
	if p == nil {
		return nil, errors.New("模型不存在")
	}
	return p, nil
}

// CreateModelRequest 创建/更新模型请求
type CreateModelRequest struct {
	Name         string  `json:"name" binding:"required"`
	APIKey       string  `json:"api_key"`
	BaseURL      string  `json:"base_url"`
	APIType      string  `json:"api_type"`
	Model        string  `json:"model"`
	CostPer1k    float64 `json:"cost_per_1k"`
	AvgLatencyMs int     `json:"avg_latency_ms"`
	QualityScore float64 `json:"quality_score"`
	MaxRPM       int     `json:"max_rpm"`
	MaxTPM       int     `json:"max_tpm"`
	Enabled      bool    `json:"enabled"`
}

// AddModel 添加模型
func (s *LLMRoutingService) AddModel(req *CreateModelRequest) (*llm.ProviderConfig, error) {
	if req.Name == "" {
		return nil, errors.New("模型名称不能为空")
	}
	if s.dispatcher.GetProvider(req.Name) != nil {
		return nil, errors.New("模型已存在")
	}
	if req.APIType == "" {
		req.APIType = "openai"
	}
	p := llm.ProviderConfig{
		Name:         req.Name,
		APIKey:       req.APIKey,
		BaseURL:      req.BaseURL,
		APIType:      req.APIType,
		Model:        req.Model,
		CostPer1k:    req.CostPer1k,
		AvgLatencyMs: req.AvgLatencyMs,
		QualityScore: req.QualityScore,
		MaxRPM:       req.MaxRPM,
		MaxTPM:       req.MaxTPM,
		Enabled:      req.Enabled,
	}
	s.dispatcher.AddProvider(p)
	return s.dispatcher.GetProvider(req.Name), nil
}

// UpdateModel 更新模型
func (s *LLMRoutingService) UpdateModel(name string, req *CreateModelRequest) (*llm.ProviderConfig, error) {
	if name == "" {
		return nil, errors.New("模型名称不能为空")
	}
	if s.dispatcher.GetProvider(name) == nil {
		return nil, errors.New("模型不存在")
	}
	p := llm.ProviderConfig{
		Name:         name,
		APIKey:       req.APIKey,
		BaseURL:      req.BaseURL,
		APIType:      req.APIType,
		Model:        req.Model,
		CostPer1k:    req.CostPer1k,
		AvgLatencyMs: req.AvgLatencyMs,
		QualityScore: req.QualityScore,
		MaxRPM:       req.MaxRPM,
		MaxTPM:       req.MaxTPM,
		Enabled:      req.Enabled,
	}
	if p.APIType == "" {
		p.APIType = "openai"
	}
	s.dispatcher.AddProvider(p)
	return s.dispatcher.GetProvider(name), nil
}

// DeleteModel 删除模型
func (s *LLMRoutingService) DeleteModel(name string) error {
	if name == "" {
		return errors.New("模型名称不能为空")
	}
	if !s.dispatcher.RemoveProvider(name) {
		return errors.New("模型不存在")
	}
	s.mu.Lock()
	delete(s.stats, name)
	s.mu.Unlock()
	return nil
}

// TestModelRequest 测试模型请求
type TestModelRequest struct {
	Prompt       string  `json:"prompt" binding:"required"`
	SystemPrompt string  `json:"system_prompt"`
	MaxTokens    int     `json:"max_tokens"`
	Temperature  float64 `json:"temperature"`
}

// TestModelResult 测试模型结果
type TestModelResult struct {
	Provider    string  `json:"provider"`
	Model       string  `json:"model"`
	Content     string  `json:"content"`
	TotalTokens int     `json:"total_tokens"`
	Cost        float64 `json:"cost"`
	LatencyMs   int     `json:"latency_ms"`
	Success     bool    `json:"success"`
	Error       string  `json:"error,omitempty"`
}

// TestModel 测试指定模型可用性
func (s *LLMRoutingService) TestModel(ctx context.Context, name string, req *TestModelRequest) (*TestModelResult, error) {
	p, err := s.GetModel(name)
	if err != nil {
		return nil, err
	}
	if !p.Enabled {
		return &TestModelResult{Provider: p.Name, Model: p.Model, Success: false, Error: "模型未启用"}, nil
	}

	dispatchReq := llm.DispatchRequest{
		Scenario:     llm.ScenarioLowCost,
		Prompt:       req.Prompt,
		SystemPrompt: req.SystemPrompt,
		MaxTokens:    req.MaxTokens,
		Temperature:  req.Temperature,
	}

	// 使用场景 low_cost，但手动指定单个 provider 直接调用，便于测试
	// 这里临时把路由指向被测 provider
	origRoute := s.dispatcher.GetRoute(llm.ScenarioLowCost)
	s.dispatcher.SetRoute(llm.ScenarioRoute{
		Scenario:   llm.ScenarioLowCost,
		Provider:   name,
		Fallbacks:  []string{},
		MaxLatency: 30000,
		MinQuality: 0,
	})
	defer func() {
		if origRoute != nil {
			s.dispatcher.SetRoute(*origRoute)
		}
	}()

	start := time.Now()
	result, derr := s.dispatcher.Dispatch(ctx, dispatchReq)
	latency := int(time.Since(start).Milliseconds())

	s.recordStat(name, p.Model, result, derr)

	if derr != nil {
		return &TestModelResult{
			Provider:  p.Name,
			Model:     p.Model,
			LatencyMs: latency,
			Success:   false,
			Error:     derr.Error(),
		}, nil
	}
	return &TestModelResult{
		Provider:    result.Provider,
		Model:       result.Model,
		Content:     result.Content,
		TotalTokens: result.TotalTokens,
		Cost:        result.Cost,
		LatencyMs:   latency,
		Success:     true,
	}, nil
}

// recordStat 记录调用统计
func (s *LLMRoutingService) recordStat(provider, model string, result *llm.DispatchResult, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stat, ok := s.stats[provider]
	if !ok {
		stat = &LLMModelStat{Provider: provider, Model: model}
		s.stats[provider] = stat
	}
	stat.CallCount++
	stat.LastUsedAt = time.Now().Unix()
	if err != nil {
		stat.FailedCount++
		return
	}
	stat.SuccessCount++
	stat.TotalTokens += int64(result.TotalTokens)
	stat.TotalCost += result.Cost
	// 滚动平均延迟
	if stat.SuccessCount == 1 {
		stat.AvgLatencyMs = int64(result.LatencyMs)
	}
}

// ListStrategies 列出所有场景路由策略
func (s *LLMRoutingService) ListStrategies() []llm.ScenarioRoute {
	list := s.dispatcher.GetRouteList()
	sort.Slice(list, func(i, j int) bool {
		return string(list[i].Scenario) < string(list[j].Scenario)
	})
	return list
}

// UpdateStrategiesRequest 批量更新路由策略请求
type UpdateStrategiesRequest struct {
	Routes []llm.ScenarioRoute `json:"routes" binding:"required"`
}

// UpdateStrategies 批量更新路由策略
func (s *LLMRoutingService) UpdateStrategies(req *UpdateStrategiesRequest) ([]llm.ScenarioRoute, error) {
	if len(req.Routes) == 0 {
		return nil, errors.New("路由策略不能为空")
	}
	for _, r := range req.Routes {
		if r.Scenario == "" {
			return nil, errors.New("场景不能为空")
		}
		if r.Provider == "" {
			return nil, errors.New("首选 provider 不能为空")
		}
		s.dispatcher.SetRoute(r)
	}
	return s.ListStrategies(), nil
}

// GetStats 获取调用统计
func (s *LLMRoutingService) GetStats() []*LLMModelStat {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]*LLMModelStat, 0, len(s.stats))
	for _, st := range s.stats {
		cp := *st
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CallCount > list[j].CallCount
	})
	return list
}

// UsageSummary 用量汇总
type UsageSummary struct {
	TotalCalls    int64   `json:"total_calls"`
	TotalSuccess  int64   `json:"total_success"`
	TotalFailed   int64   `json:"total_failed"`
	TotalTokens   int64   `json:"total_tokens"`
	TotalCost     float64 `json:"total_cost"`
	ActiveModels  int     `json:"active_models"`
	EnabledModels int     `json:"enabled_models"`
}

// GetUsage 获取用量汇总
func (s *LLMRoutingService) GetUsage() *UsageSummary {
	stats := s.GetStats()
	summary := &UsageSummary{}
	for _, st := range stats {
		summary.TotalCalls += st.CallCount
		summary.TotalSuccess += st.SuccessCount
		summary.TotalFailed += st.FailedCount
		summary.TotalTokens += st.TotalTokens
		summary.TotalCost += st.TotalCost
		if st.CallCount > 0 {
			summary.ActiveModels++
		}
	}
	for _, p := range s.dispatcher.GetProviderList() {
		if p.Enabled {
			summary.EnabledModels++
		}
	}
	return summary
}
