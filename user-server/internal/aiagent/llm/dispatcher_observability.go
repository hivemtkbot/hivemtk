package llm

import (
	"context"
	"hash/fnv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils/logger"
)


const (
	TokenSourceActual    = "actual"
	TokenSourceEstimated = "estimated"
	TokenSourceMissing   = "missing"

	ModelTypeLocal = "local"
	ModelTypeCloud = "cloud"

	SourceDispatch = "dispatch"
	SourceCache    = "cache"
	SourceFallback = "fallback"
	SourceEmpty    = "null"

	EstimatorCharWeight    = "char_weight"
	EstimatorEmptyFallback = "empty_fallback"
)

// missingCounter 全局 missing 计数器（用于占比告警）
var (
	missingCounter   int64
	totalCounter     int64
	missingThreshold = int64(20)
)


// InferVendor 根据 BaseURL 推断厂商
//
// 与 service/llm_routing_service.go 的 vendorFromBaseURL 保持一致，
// 避免 service → llm 包的反向依赖。
func InferVendor(baseURL string) string {
	low := strings.ToLower(baseURL)
	switch {
	case strings.Contains(low, "deepseek"):
		return "deepseek"
	case strings.Contains(low, "dashscope"), strings.Contains(low, "qwen"):
		return "qwen"
	case strings.Contains(low, "openai"), strings.Contains(low, "gpt"):
		return "openai"
	case strings.Contains(low, "zhipu"), strings.Contains(low, "glm"), strings.Contains(low, "bigmodel"):
		return "zhipu"
	case strings.Contains(low, "moonshot"), strings.Contains(low, "kimi"):
		return "moonshot"
	case strings.Contains(low, "127.0.0.1"), strings.Contains(low, "localhost"), strings.Contains(low, "mtk-llm"):
		return "local"
	default:
		return "other"
	}
}

// InferModelType 根据 BaseURL 推断模型类型（local / cloud）
func InferModelType(baseURL string) string {
	low := strings.ToLower(baseURL)
	if low == "" || strings.Contains(low, "127.0.0.1") || strings.Contains(low, "localhost") || strings.Contains(low, "mtk-llm") {
		return ModelTypeLocal
	}
	return ModelTypeCloud
}

// InferTokenSource 根据 LLM 真实 Usage 判定 token_source
//
//   - TotalTokens > 0 → actual（LLM 返回了真实 usage）
//   - TotalTokens == 0 且有 content → estimated（需要字符估算兜底）
//   - 无 content 且无 usage → missing（响应异常）
func InferTokenSource(totalTokens int, content string) string {
	if totalTokens > 0 {
		return TokenSourceActual
	}
	if content != "" {
		return TokenSourceEstimated
	}
	return TokenSourceMissing
}

// ClassifyEstimator 根据估算路径标记 estimator 字段
func ClassifyEstimator(tokenSource string) string {
	switch tokenSource {
	case TokenSourceEstimated:
		return EstimatorCharWeight
	case TokenSourceMissing:
		return EstimatorEmptyFallback
	default:
		return ""
	}
}


// auditDB 全局审计 DB 句柄（由 InitGlobalDispatcher 注入）
var (
	auditDBMu sync.RWMutex
	auditDB   *gorm.DB
)

// setAuditDB 设置审计 DB（main 启动时调用）
func setAuditDB(d *gorm.DB) {
	auditDBMu.Lock()
	defer auditDBMu.Unlock()
	auditDB = d
}

// getAuditDB 获取审计 DB
func getAuditDB() *gorm.DB {
	auditDBMu.RLock()
	defer auditDBMu.RUnlock()
	return auditDB
}

// AttachAuditDB 注入审计 DB（公开 API，main.go 在 InitGlobalDispatcher 后调用）
func AttachAuditDB(d *gorm.DB) {
	setAuditDB(d)
}


// LogEntry 路由决策日志条目（包含 v3.6.0 基础字段 + v3.7.0 扩展字段）
//
// 设计原则：
//   - 一份结构体承载所有字段，避免函数参数爆炸
//   - 调用方通过 NewLogEntry 构造，保证扩展字段自动填充
//   - 落库失败仅记日志，不阻塞业务
type LogEntry struct {
	TraceID          string           `json:"trace_id"`
	Scenario         DispatchScenario `json:"scenario"`
	Provider         string           `json:"provider"`
	Model            string           `json:"model"`
	PromptTokens     int              `json:"prompt_tokens"`
	CompletionTokens int              `json:"completion_tokens"`
	TotalTokens      int              `json:"total_tokens"`
	Cost             float64          `json:"cost"`
	LatencyMs        int              `json:"latency_ms"`
	Success          bool             `json:"success"`
	ErrorMsg         string           `json:"error_msg"`
	FromCache        bool             `json:"from_cache"`
	ModelType        string  `json:"model_type"`        
	Vendor           string  `json:"vendor"`            
	BaseURL          string  `json:"base_url"`          
	IsFallback       bool    `json:"is_fallback"`       
	PromptCost       float64 `json:"prompt_cost"`       
	CompletionCost   float64 `json:"completion_cost"`   
	TokenSource      string  `json:"token_source"`      
	Estimator        string  `json:"estimator"`         
	Source           string  `json:"source"`            
	ScenarioProvider string  `json:"scenario_provider"` 
	InternalLang    string `json:"internal_lang,omitempty"`    
	TargetLang      string `json:"target_lang,omitempty"`      
	CrossLingual    bool   `json:"cross_lingual,omitempty"`    
	GlossaryVersion string `json:"glossary_version,omitempty"` 
	CacheHit        bool   `json:"cache_hit,omitempty"`        
}

// NewLogEntry 根据基础信息构造 LogEntry，自动填充扩展字段
//
// 参数：
//   - scenario:    调度场景
//   - provider:    命中的 provider 配置（用于推断 vendor/model_type/base_url）
//   - model:       实际调用的 model 名
//   - usage:       LLM 真实 token 用量（PromptTokens/CompletionTokens/TotalTokens）
//   - cost:        总成本（用于 cost 字段，prompt/completion 拆分按 token 比例计算）
//   - latencyMs:   调用耗时
//   - success:     是否成功
//   - errMsg:      失败原因
//   - fromCache:   是否命中缓存
//   - isFallback:  是否为降级调用
//   - traceID:     请求 trace_id
//   - content:     LLM 响应内容（用于判定 token_source 是否 estimated）
//   - source:      调用来源（dispatch/cache/fallback/null）
func NewLogEntry(scenario DispatchScenario, provider *ProviderConfig, model string,
	promptTokens, completionTokens, totalTokens int, cost float64, latencyMs int,
	success bool, errMsg string, fromCache, isFallback bool, traceID, content, source string) *LogEntry {

	baseURL := ""
	if provider != nil {
		baseURL = provider.BaseURL
	}
	tokenSource := InferTokenSource(totalTokens, content)
	estimator := ClassifyEstimator(tokenSource)
	modelType := InferModelType(baseURL)
	vendor := InferVendor(baseURL)

	promptCost, completionCost := splitCost(cost, promptTokens, completionTokens, totalTokens)

	if fromCache {
		source = SourceCache
	} else if isFallback {
		// v3 审计 P2-6 修复：isFallback 路径 source 应为 SourceFallback
		// 原：仅 fromCache 改 source，isFallback 路径仍记 SourceDispatch
		//      降级占比无法统计
		source = SourceFallback
	}

	return &LogEntry{
		TraceID:          traceID,
		Scenario:         scenario,
		Provider:         providerName(provider),
		Model:            model,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		Cost:             cost,
		LatencyMs:        latencyMs,
		Success:          success,
		ErrorMsg:         errMsg,
		FromCache:        fromCache,
		ModelType:        modelType,
		Vendor:           vendor,
		BaseURL:          baseURL,
		IsFallback:       isFallback,
		PromptCost:       promptCost,
		CompletionCost:   completionCost,
		TokenSource:      tokenSource,
		Estimator:        estimator,
		Source:           source,
		ScenarioProvider: string(scenario) + "|" + providerName(provider),
	}
}

// providerName 安全获取 provider 名
func providerName(p *ProviderConfig) string {
	if p == nil {
		return ""
	}
	return p.Name
}

// splitCost 按 prompt/completion token 比例拆分总成本
//
// 当 totalTokens == 0 时，全部计入 prompt_cost（避免除零）。
// 本地模型 cost 通常为 0（CostPer1k=0），拆分后仍为 0。
func splitCost(totalCost float64, promptTokens, completionTokens, totalTokens int) (promptCost, completionCost float64) {
	if totalTokens <= 0 {
		return totalCost, 0
	}
	ratio := float64(promptTokens) / float64(totalTokens)
	promptCost = totalCost * ratio
	completionCost = totalCost - promptCost
	return
}

// LogRoutingDecision 记录一次 dispatch 决策（v3.7.0 重构版）
//
// 调用方优先使用 NewLogEntry 构造 entry，再传入本函数。
// 落库失败仅记录日志，不阻塞业务。
// 同时维护 missing 计数器，超过阈值时触发告警。
func LogRoutingDecision(ctx context.Context, entry *LogEntry) {
	if entry == nil {
		return
	}
	d := getAuditDB()
	if d == nil {
		updateMissingCounter(entry)
		return
	}
	row := map[string]any{
		"trace_id":          entry.TraceID,
		"scenario":          string(entry.Scenario),
		"provider":          entry.Provider,
		"model":             entry.Model,
		"prompt_tokens":     entry.PromptTokens,
		"completion_tokens": entry.CompletionTokens,
		"total_tokens":      entry.TotalTokens,
		"cost":              entry.Cost,
		"latency_ms":        entry.LatencyMs,
		"success":           entry.Success,
		"error_msg":         entry.ErrorMsg,
		"from_cache":        entry.FromCache,
		"model_type":        entry.ModelType,
		"vendor":            entry.Vendor,
		"base_url":          entry.BaseURL,
		"is_fallback":       entry.IsFallback,
		"prompt_cost":       entry.PromptCost,
		"completion_cost":   entry.CompletionCost,
		"token_source":      entry.TokenSource,
		"estimator":         entry.Estimator,
		"source":            entry.Source,
		"scenario_provider": entry.ScenarioProvider,
		"internal_lang":    entry.InternalLang,
		"target_lang":      entry.TargetLang,
		"cross_lingual":    entry.CrossLingual,
		"glossary_version": entry.GlossaryVersion,
		"cache_hit":        entry.CacheHit,
	}
	if err := d.WithContext(context.Background()).Table("llm_routing_logs").Create(row).Error; err != nil {
		logger.Warnf("[LLM] LogRoutingDecision write failed: %v (entry=%+v)", err, entry)
	}
	updateMissingCounter(entry)
}

// updateMissingCounter 更新 missing 计数器并检查阈值
//
// 每 100 次调用检查一次 missing 占比，超过阈值时打印告警日志。
// 计数器为进程级，重启后归零。
//
// 注意：source=cache 的缓存命中不计入统计——缓存命中无 LLM API 调用，
// 不影响 missing 占比（missing 占比仅监控真实 LLM API 响应完整性）。
func updateMissingCounter(entry *LogEntry) {
	if entry == nil {
		return
	}
	if entry.Source == SourceCache {
		return
	}
	total := atomic.AddInt64(&totalCounter, 1)
	if entry.TokenSource == TokenSourceMissing {
		atomic.AddInt64(&missingCounter, 1)
	}
	if total%100 == 0 {
		missing := atomic.LoadInt64(&missingCounter)
		if total > 0 {
			ratio := missing * 100 / total
			if ratio >= missingThreshold {
				logger.Errorf("[LLM] token_source=missing 占比告警: %d/%d (%d%%) >= %d%%阈值，请检查 LLM API 响应完整性",
					missing, total, ratio, missingThreshold)
			}
		}
	}
}

// GetTokenSourceStats 获取进程内 token_source 统计（供监控 API 查询）
func GetTokenSourceStats() (total, missing int64) {
	return atomic.LoadInt64(&totalCounter), atomic.LoadInt64(&missingCounter)
}

// ResetTokenSourceStats 重置计数器（仅供测试使用）
func ResetTokenSourceStats() {
	atomic.StoreInt64(&totalCounter, 0)
	atomic.StoreInt64(&missingCounter, 0)
}

// ScenarioStat 场景维度统计（用于 Usage API）
type ScenarioStat struct {
	Scenario     string  `json:"scenario"`
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	CallCount    int64   `json:"call_count"`
	SuccessCount int64   `json:"success_count"`
	FailedCount  int64   `json:"failed_count"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalCost    float64 `json:"total_cost"`
	AvgLatencyMs int64   `json:"avg_latency_ms"`
	WindowLabel  string  `json:"window_label"` 
}

// QueryScenarioStats 查 llm_routing_logs 按 (scenario, provider) 聚合
//
// 参数：
//   - window: 时间窗口（"today" / "week" / "month" / "all"），默认 "all"
//   - limit:  最多返回多少条聚合（默认 200）
//   - enabledProviders: 只统计这些 provider（为空则不过滤）
func QueryScenarioStats(ctx context.Context, window string, limit int, enabledProviders []string) ([]ScenarioStat, error) {
	d := getAuditDB()
	if d == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 200
	}
	var since time.Time
	now := time.Now()
	switch window {
	case "today":
		since = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	case "week":
		since = now.AddDate(0, 0, -7)
	case "month":
		since = now.AddDate(0, -1, 0)
	default:
		since = time.Time{}
	}

	q := d.WithContext(ctx).Table("llm_routing_logs").
		Select(`scenario, provider, MAX(model) AS model,
				COUNT(*) AS call_count,
				SUM(CASE WHEN success THEN 1 ELSE 0 END) AS success_count,
				SUM(CASE WHEN success THEN 0 ELSE 1 END) AS failed_count,
				COALESCE(SUM(total_tokens),0) AS total_tokens,
				COALESCE(SUM(cost),0) AS total_cost,
				COALESCE(AVG(latency_ms),0)::bigint AS avg_latency_ms`).
		Group("scenario, provider").
		Order("call_count DESC").
		Limit(limit)
	if !since.IsZero() {
		q = q.Where("created_at >= ?", since)
	}
	if len(enabledProviders) > 0 {
		q = q.Where("provider IN ?", enabledProviders)
	}
	var rows []ScenarioStat
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].WindowLabel = window
	}
	return rows, nil
}

// QueryAuditHistory 查路由变更历史
func QueryAuditHistory(ctx context.Context, scenario string, limit int) ([]map[string]any, error) {
	d := getAuditDB()
	if d == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	q := d.WithContext(ctx).Table("llm_routing_audit").
		Order("created_at DESC").
		Limit(limit)
	if scenario != "" {
		q = q.Where("scenario = ?", scenario)
	}
	var rows []map[string]any
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}


// DecideCanaryRoute 决定本次 Dispatch 走哪个 route（主或灰度）
//
// 决策规则：
//   - Weight = 0：返回 nil（灰度未启用，走主路由）
//   - Weight = 100：返回 CanaryRoute（全部走灰度）
//   - 0 < Weight < 100：
//     1) 若 CanaryKey 非空 → 用 fnv32(canaryKey) % 100 与 Weight 比较
//     2) 若 CanaryKey 为空 → 用 time.Now().UnixNano() % 100 与 Weight 比较
//   - 返回 nil 时走主 route；返回非 nil 时走 canary route
func DecideCanaryRoute(route *ScenarioRoute, canaryKey string) *ScenarioRoute {
	if route == nil || route.Weight <= 0 || route.CanaryRoute == nil {
		return nil
	}
	if route.Weight >= 100 {
		return route.CanaryRoute
	}
	var bucket uint64
	if canaryKey != "" {
		h := fnv.New64a()
		_, _ = h.Write([]byte(canaryKey))
		bucket = h.Sum64() % 100
	} else {
		bucket = uint64(time.Now().UnixNano() % 100)
	}
	if bucket < uint64(route.Weight) {
		return route.CanaryRoute
	}
	return nil
}


// StartCacheJanitor 启动后台 goroutine 定期清理过期 cache 项
//
// 间隔：60 秒一次；可调用返回的 stop 函数终止。
// 解决缺陷 10：cache map 只增不减导致 OOM 风险。
func (d *Dispatcher) StartCacheJanitor(ctx context.Context, interval time.Duration) (stop func()) {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	ticker := time.NewTicker(interval)
	stopCh := make(chan struct{})
	var once sync.Once
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-stopCh:
				return
			case <-ticker.C:
				d.sweepExpiredCache()
			}
		}
	}()
	return func() {
		once.Do(func() {
			close(stopCh)
			ticker.Stop()
		})
	}
}

// sweepExpiredCache 响应缓存已迁至全局缓存（REDIS_HOST 配置时 Redis 共享、否则内存单例），
// 由全局缓存按 TTL 自动过期，无需手动全扫。保留签名以兼容既有调用点。
func (d *Dispatcher) sweepExpiredCache() {}

// InitGlobalDispatcherWithDB 初始化全局 Dispatcher 并注入审计 DB
//
// main.go 启动期调用，把 gorm DB 注入以支持 SetRouteWithAudit / LogRoutingDecision。
func InitGlobalDispatcherWithDB(d *Dispatcher, gormDB *gorm.DB) {
	InitGlobalDispatcher(d)
	setAuditDB(gormDB)
	if getAuditDB() == nil {
		setAuditDB(db.GetDB())
	}
}

