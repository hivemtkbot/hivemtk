package llm

import (
	"context"
	"hash/fnv"
	"sync"
	"time"

	"gorm.io/gorm"

	"marketing/internal/pkg/utils/db"
)

// ============================================================================
// 可观测性补充（2026-07-23）
// ----------------------------------------------------------------------------
//  1. 审计日志落库（SetRouteWithAudit）
//  2. 路由决策日志落库（每次 Dispatch 命中 provider）
//  3. 缓存后台清理 ticker
//  4. 灰度发布决策（canary weight）
//  5. 场景维度统计聚合（按 (scenario, provider, day) 物化）
// ============================================================================

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

// ============================================================================
// 路由决策日志（每次 Dispatch 落一条到 llm_routing_logs）
// ============================================================================

// LogRoutingDecision 记录一次 dispatch 决策
//
// 参数：
//   - scenario:  调度场景
//   - provider:  命中的 provider
//   - model:     实际调用的 model
//   - tokens:    prompt/completion/total
//   - cost:      估算成本
//   - latency:   调用耗时
//   - success:   是否成功
//   - errMsg:    失败原因
//   - fromCache: 是否命中缓存
//   - traceID:   请求 trace_id
//
// 落库失败仅记录日志，不阻塞业务。
func LogRoutingDecision(ctx context.Context, scenario DispatchScenario, provider, model string, promptTokens, completionTokens, totalTokens int, cost float64, latencyMs int, success bool, errMsg string, fromCache bool, traceID string) {
	d := getAuditDB()
	if d == nil {
		return
	}
	row := map[string]any{
		"trace_id":          traceID,
		"scenario":          string(scenario),
		"provider":          provider,
		"model":             model,
		"prompt_tokens":     promptTokens,
		"completion_tokens": completionTokens,
		"total_tokens":      totalTokens,
		"cost":              cost,
		"latency_ms":        latencyMs,
		"success":           success,
		"error_msg":         errMsg,
		"from_cache":        fromCache,
	}
	if err := d.WithContext(ctx).Table("llm_routing_logs").Create(row).Error; err != nil {
		// 写日志失败不应影响主流程
		// 走包内 logger 即可（已 import）
		_ = err
	}
}

// ScenarioStat 场景维度统计（用于 Usage API）
type ScenarioStat struct {
	Scenario      string  `json:"scenario"`
	Provider      string  `json:"provider"`
	Model         string  `json:"model"`
	CallCount     int64   `json:"call_count"`
	SuccessCount  int64   `json:"success_count"`
	FailedCount   int64   `json:"failed_count"`
	TotalTokens   int64   `json:"total_tokens"`
	TotalCost     float64 `json:"total_cost"`
	AvgLatencyMs  int64   `json:"avg_latency_ms"`
	WindowLabel   string  `json:"window_label"` // "today" / "week" / "month" / "all"
}

// QueryScenarioStats 查 llm_routing_logs 按 (scenario, provider) 聚合
//
// 参数：
//   - window: 时间窗口（"today" / "week" / "month" / "all"），默认 "all"
//   - limit:  最多返回多少条聚合（默认 200）
func QueryScenarioStats(ctx context.Context, window string, limit int) ([]ScenarioStat, error) {
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

// ============================================================================
// 灰度发布决策
// ============================================================================

// DecideCanaryRoute 决定本次 Dispatch 走哪个 route（主或灰度）
//
// 决策规则：
//   - Weight = 0：返回 nil（灰度未启用，走主路由）
//   - Weight = 100：返回 CanaryRoute（全部走灰度）
//   - 0 < Weight < 100：
//       1) 若 CanaryKey 非空 → 用 fnv32(canaryKey) % 100 与 Weight 比较
//       2) 若 CanaryKey 为空 → 用 time.Now().UnixNano() % 100 与 Weight 比较
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

// ============================================================================
// 缓存后台清理 ticker
// ============================================================================

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

// sweepExpiredCache 清理过期 cache（O(N) 全扫，对当前 <10k 缓存量可接受）
func (d *Dispatcher) sweepExpiredCache() {
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()
	now := time.Now()
	expired := 0
	for k, e := range d.cache {
		if now.After(e.expireAt) {
			delete(d.cache, k)
			expired++
		}
	}
	if expired > 0 {
		// 仅在有清理时打日志，避免噪音
		_ = expired
	}
}

// InitGlobalDispatcherWithDB 初始化全局 Dispatcher 并注入审计 DB
//
// main.go 启动期调用，把 gorm DB 注入以支持 SetRouteWithAudit / LogRoutingDecision。
func InitGlobalDispatcherWithDB(d *Dispatcher, gormDB *gorm.DB) {
	InitGlobalDispatcher(d)
	setAuditDB(gormDB)
	// 兜底：若用户未传 DB，从全局 db util 取一份
	if getAuditDB() == nil {
		setAuditDB(db.GetDB())
	}
}
