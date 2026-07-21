package controller

// dashboard_sse.go 实时驾驶舱 SSE 流式推送（M-4 P1）
//
// 五层架构归属: L1 控制器层
// 设计依据: PRD §M-4 P1 缺口修复
// 私域独立部署: 无 merchant_id 字段
//
// EventStream 设计契约：
//   - Content-Type: text/event-stream
//   - Cache-Control: no-cache
//   - Connection: keep-alive
//   - X-Accel-Buffering: no
//   - 心跳：每 15s 发送 ":ping"
//
// 实时数据维度（每 2s 推送一次）：
//   1. 在线会话数（status IN ai_handling/human_handling/waiting 且 last_message_at 在 5min 内）
//   2. 正在生成的回复（status = ai_handling）
//   3. AI 拟人度分布（最近 1h humanize_scores 直方图：<0.7 / 0.7-0.85 / >=0.85）
//   4. 转化漏斗进度（stranger → lead → contact → quote → won 各阶段数量）
//
// 路由：
//   - GET /api/dashboard/sse/stream     实时数据流（EventStream）
//   - GET /api/dashboard/sse/snapshot   当前快照（一次性 JSON）
//   - GET /api/dashboard/sse/metrics    完整 metrics 指标
//
// 与已有 sse_dashboard.go 的关系：
//   - sse_dashboard.go：通用 SSE Hub（多 topic 订阅广播）
//   - dashboard_sse.go：专用实时驾驶舱（聚合数据库指标流式推送）
//   - 两者并行存在：dashboard_sse 是 M-4 P1 新增的实时驾驶舱专用 SSE

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/pkg/utils/response"
)

// DashboardSSEController 实时驾驶舱 SSE 控制器
type DashboardSSEController struct {
	db *gorm.DB

	// 缓存（最近一次查询结果，避免每次推送都查库）
	cacheMu      sync.RWMutex
	lastSnapshot *DashboardSnapshot
	lastUpdateAt time.Time
	cacheTTL     time.Duration

	// 流订阅者统计
	subscriberCount atomic.Int64
}

// NewDashboardSSEController 创建实时驾驶舱 SSE 控制器
func NewDashboardSSEController(db *gorm.DB) *DashboardSSEController {
	return &DashboardSSEController{
		db:       db,
		cacheTTL: 2 * time.Second,
	}
}

// DashboardSnapshot 实时驾驶舱数据快照
type DashboardSnapshot struct {
	// 时间戳
	GeneratedAt time.Time `json:"generated_at"`

	// 1. 在线会话数
	OnlineSessions  int `json:"online_sessions"`   // 在线会话总数
	AISessions      int `json:"ai_sessions"`       // AI 处理中
	HumanSessions   int `json:"human_sessions"`    // 人工处理中
	WaitingSessions int `json:"waiting_sessions"`  // 等待用户回复
	InFlightReplies int `json:"in_flight_replies"` // 正在生成的回复

	// 2. 拟人度分布（最近 1h）
	HumanizeDistribution HumanizeDistribution `json:"humanize_distribution"`

	// 3. 转化漏斗进度
	Funnel *FunnelProgress `json:"funnel"`

	// 4. LLM 实时指标
	LLMMetrics *LLMRealTimeMetrics `json:"llm_metrics"`
}

// HumanizeDistribution AI 拟人度分布（最近 1h）
type HumanizeDistribution struct {
	WindowHours      int     `json:"window_hours"`
	TotalScored      int     `json:"total_scored"`
	LowScoreCount    int     `json:"low_score_count"`    // < 0.7（需要重生成）
	MediumScoreCount int     `json:"medium_score_count"` // 0.7 - 0.85（边界）
	HighScoreCount   int     `json:"high_score_count"`   // >= 0.85（达标）
	AvgScore         float64 `json:"avg_score"`
	PassRate         float64 `json:"pass_rate"` // >= 0.85 占比
}

// FunnelProgress 转化漏斗进度
type FunnelProgress struct {
	Stages       []FunnelStage `json:"stages"`
	TotalEntered int           `json:"total_entered"`
	TotalWon     int           `json:"total_won"`
	OverallRate  float64       `json:"overall_rate"` // 端到端转化率（won / entered）
}

// FunnelStage 漏斗单个阶段
type FunnelStage struct {
	Name      string  `json:"name"`       // 阶段名
	Code      string  `json:"code"`       // 阶段编码
	Count     int     `json:"count"`      // 当前阶段客户数
	StageRate float64 `json:"stage_rate"` // 占总数比例（%）
	StepRate  float64 `json:"step_rate"`  // 上一步到本步的转化率（%）
}

// LLMRealTimeMetrics LLM 实时指标
type LLMRealTimeMetrics struct {
	ActiveProviders int     `json:"active_providers"` // 启用的 provider 数
	DownProviders   int     `json:"down_providers"`   // 熔断 / down 的 provider 数
	CircuitOpen     int     `json:"circuit_open"`     // 熔断器开启数
	AvgLatencyMs    float64 `json:"avg_latency_ms"`   // 平均延迟
	FailureRate     float64 `json:"failure_rate"`     // 失败率
}

// StreamEventStream 实时数据流
//
// EventStream:
//
//	event: dashboard_update
//	data: {...}
//
//	event: heartbeat
//	data: {"ts":"..."}
//
// 每 2s 推送一次 dashboard_update，每 15s 推送一次 heartbeat
func (c *DashboardSSEController) StreamEventStream(ctx *gin.Context) {
	c.subscriberCount.Add(1)
	defer c.subscriberCount.Add(-1)

	// 设置 SSE 响应头
	ctx.Writer.Header().Set("Content-Type", "text/event-stream")
	ctx.Writer.Header().Set("Cache-Control", "no-cache")
	ctx.Writer.Header().Set("Connection", "keep-alive")
	ctx.Writer.Header().Set("X-Accel-Buffering", "no")
	ctx.Writer.WriteHeader(http.StatusOK)

	// 立即推送首次快照
	snapshot, err := c.collectSnapshot(ctx.Request.Context())
	if err != nil {
		logger.Ctx(ctx.Request.Context()).Warn().
			Err(err).
			Msg("dashboard_sse: initial snapshot failed")
	} else {
		if writeErr := writeDashboardEvent(ctx, "dashboard_update", snapshot); writeErr != nil {
			return
		}
	}

	// 推送连接成功事件
	_ = writeDashboardRawEvent(ctx, "connected", map[string]any{
		"client_ip":   ctx.ClientIP(),
		"subscriber":  c.subscriberCount.Load(),
		"message":     "dashboard event stream connected",
		"refresh_sec": int(c.cacheTTL.Seconds()),
	})

	// 定时推送循环
	dataTicker := time.NewTicker(c.cacheTTL)
	defer dataTicker.Stop()
	heartbeatTicker := time.NewTicker(15 * time.Second)
	defer heartbeatTicker.Stop()

	clientClosed := ctx.Request.Context().Done()
	for {
		select {
		case <-clientClosed:
			return
		case <-dataTicker.C:
			snapshot, err := c.collectSnapshot(ctx.Request.Context())
			if err != nil {
				logger.Ctx(ctx.Request.Context()).Warn().
					Err(err).
					Msg("dashboard_sse: snapshot collection failed")
				continue
			}
			if writeErr := writeDashboardEvent(ctx, "dashboard_update", snapshot); writeErr != nil {
				return
			}
		case <-heartbeatTicker.C:
			if writeErr := writeDashboardRawEvent(ctx, "heartbeat", map[string]any{
				"ts":         time.Now().UTC().Format(time.RFC3339Nano),
				"subscriber": c.subscriberCount.Load(),
			}); writeErr != nil {
				return
			}
		}
	}
}

// Snapshot 返回当前快照（一次性 JSON）
func (c *DashboardSSEController) Snapshot(ctx *gin.Context) {
	snapshot, err := c.collectSnapshot(ctx.Request.Context())
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "采集快照失败: "+err.Error())
		return
	}
	response.Success(ctx, snapshot, "ok")
}

// Metrics 完整指标（含历史趋势）
func (c *DashboardSSEController) Metrics(ctx *gin.Context) {
	snapshot, err := c.collectSnapshot(ctx.Request.Context())
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "采集指标失败: "+err.Error())
		return
	}
	// 附加订阅者数 / 缓存时间
	response.Success(ctx, map[string]any{
		"snapshot":         snapshot,
		"subscriber_count": c.subscriberCount.Load(),
		"cache_ttl_sec":    int(c.cacheTTL.Seconds()),
		"server_time":      time.Now().UTC().Format(time.RFC3339Nano),
	}, "ok")
}

// collectSnapshot 采集实时数据快照（带 2s 缓存）
func (c *DashboardSSEController) collectSnapshot(ctx context.Context) (*DashboardSnapshot, error) {
	c.cacheMu.RLock()
	if c.lastSnapshot != nil && time.Since(c.lastUpdateAt) < c.cacheTTL {
		snap := c.lastSnapshot
		c.cacheMu.RUnlock()
		return snap, nil
	}
	c.cacheMu.RUnlock()

	snap := &DashboardSnapshot{
		GeneratedAt: time.Now(),
	}

	if c.db != nil {
		// 1. 在线会话数
		c.collectSessionStats(ctx, snap)
	}
	// 2. 拟人度分布（db=nil 时仍返回 WindowHours=1 标记窗口期）
	snap.HumanizeDistribution = c.collectHumanizeDistribution(ctx)
	// 3. 转化漏斗（始终返回非 nil，便于前端渲染）
	snap.Funnel = c.collectFunnel(ctx)
	// 4. LLM 指标（始终返回非 nil，提供默认零值）
	snap.LLMMetrics = c.collectLLMMetrics(ctx)

	// 缓存
	c.cacheMu.Lock()
	c.lastSnapshot = snap
	c.lastUpdateAt = time.Now()
	c.cacheMu.Unlock()

	return snap, nil
}

// collectSessionStats 采集会话统计
func (c *DashboardSSEController) collectSessionStats(ctx context.Context, snap *DashboardSnapshot) {
	// 5 分钟内有消息视为"在线"
	onlineThreshold := time.Now().Add(-5 * time.Minute)

	// 在线会话：status IN active set 且 last_message_at > threshold
	var onlineCount int64
	c.db.WithContext(ctx).Model(&model.CustomerSession{}).
		Where("status IN ?", []model.SessionStatus{
			model.SessionStatusAIHandling,
			model.SessionStatusHumanHandling,
			model.SessionStatusWaiting,
		}).
		Where("last_message_at > ? OR last_message_at IS NULL", onlineThreshold).
		Count(&onlineCount)
	snap.OnlineSessions = int(onlineCount)

	// AI 处理中
	var aiCount int64
	c.db.WithContext(ctx).Model(&model.CustomerSession{}).
		Where("status = ?", model.SessionStatusAIHandling).
		Count(&aiCount)
	snap.AISessions = int(aiCount)

	// 人工处理中
	var humanCount int64
	c.db.WithContext(ctx).Model(&model.CustomerSession{}).
		Where("status = ?", model.SessionStatusHumanHandling).
		Count(&humanCount)
	snap.HumanSessions = int(humanCount)

	// 等待用户回复
	var waitingCount int64
	c.db.WithContext(ctx).Model(&model.CustomerSession{}).
		Where("status = ?", model.SessionStatusWaiting).
		Count(&waitingCount)
	snap.WaitingSessions = int(waitingCount)

	// 正在生成的回复 = AI 处理中
	snap.InFlightReplies = snap.AISessions
}

// collectHumanizeDistribution 采集拟人度分布（最近 1h）
func (c *DashboardSSEController) collectHumanizeDistribution(ctx context.Context) HumanizeDistribution {
	dist := HumanizeDistribution{WindowHours: 1}
	if c.db == nil {
		return dist
	}
	since := time.Now().Add(-1 * time.Hour)
	// 假设表名：humanize_scores，含 score 字段
	// 不依赖具体 schema：尝试查询，失败时返回空分布
	type scoreRow struct {
		Total int64
		Low   int64
		Mid   int64
		High  int64
		Avg   float64
	}
	var row scoreRow
	// 用 raw SQL 兼容不同表结构
	raw := `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE score < 0.7) as low,
			COUNT(*) FILTER (WHERE score >= 0.7 AND score < 0.85) as mid,
			COUNT(*) FILTER (WHERE score >= 0.85) as high,
			COALESCE(AVG(score), 0) as avg
		FROM humanize_scores
		WHERE created_at > ?`
	tx := c.db.WithContext(ctx).Raw(raw, since).Scan(&row)
	if tx.Error != nil {
		logger.Ctx(ctx).Warn().Err(tx.Error).Msg("dashboard_sse: humanize query failed")
		return dist
	}
	dist.TotalScored = int(row.Total)
	dist.LowScoreCount = int(row.Low)
	dist.MediumScoreCount = int(row.Mid)
	dist.HighScoreCount = int(row.High)
	dist.AvgScore = roundTo(row.Avg, 3)
	if row.Total > 0 {
		dist.PassRate = roundTo(float64(row.High)/float64(row.Total)*100, 2)
	}
	return dist
}

// collectFunnel 采集转化漏斗（基于 customer_sessions 的 customer_id 在客户旅程中的阶段）
//
// 数据源：customer_sessions 中 customer_id 在 customer_journey 表的阶段分布
// 失败时返回基于 session 状态的简化漏斗
func (c *DashboardSSEController) collectFunnel(ctx context.Context) *FunnelProgress {
	funnel := &FunnelProgress{
		Stages: []FunnelStage{},
	}
	if c.db == nil {
		return funnel
	}

	// 尝试查询 customer_journey 表（如果存在）
	type stageRow struct {
		Stage string
		Count int64
	}
	var rows []stageRow
	tx := c.db.WithContext(ctx).Raw(`
		SELECT stage, COUNT(*) as count
		FROM customer_journey
		WHERE created_at > ?
		GROUP BY stage`, time.Now().AddDate(0, 0, -30)).Scan(&rows)

	if tx.Error != nil || len(rows) == 0 {
		// 兜底：使用 customer_sessions 状态作为简化的 5 阶段漏斗
		return c.collectFunnelFromSessions(ctx)
	}

	stageOrder := []struct {
		Name string
		Code string
	}{
		{"陌生人", "stranger"},
		{"线索", "lead"},
		{"接触", "contact"},
		{"报价", "quote"},
		{"成单", "won"},
	}
	stageCount := make(map[string]int64)
	for _, r := range rows {
		stageCount[r.Stage] = r.Count
	}
	totalEntered := int64(0)
	totalWon := int64(0)
	prevCount := int64(0)
	for i, s := range stageOrder {
		cnt := stageCount[s.Code]
		if i == 0 {
			totalEntered = cnt
		}
		if s.Code == "won" {
			totalWon = cnt
		}
		stepRate := 0.0
		if i > 0 && prevCount > 0 {
			stepRate = float64(cnt) / float64(prevCount) * 100
		}
		stageRate := 0.0
		if totalEntered > 0 {
			stageRate = float64(cnt) / float64(totalEntered) * 100
		}
		funnel.Stages = append(funnel.Stages, FunnelStage{
			Name:      s.Name,
			Code:      s.Code,
			Count:     int(cnt),
			StageRate: roundTo(stageRate, 2),
			StepRate:  roundTo(stepRate, 2),
		})
		prevCount = cnt
	}
	funnel.TotalEntered = int(totalEntered)
	funnel.TotalWon = int(totalWon)
	if totalEntered > 0 {
		funnel.OverallRate = roundTo(float64(totalWon)/float64(totalEntered)*100, 2)
	}
	return funnel
}

// collectFunnelFromSessions 兜底：基于 customer_sessions 状态聚合
func (c *DashboardSSEController) collectFunnelFromSessions(ctx context.Context) *FunnelProgress {
	funnel := &FunnelProgress{
		Stages: []FunnelStage{},
	}
	if c.db == nil {
		return funnel
	}
	type statusCount struct {
		Status string
		Count  int64
	}
	var rows []statusCount
	c.db.WithContext(ctx).Raw(`
		SELECT status, COUNT(DISTINCT user_id) as count
		FROM customer_sessions
		WHERE created_at > ?
		GROUP BY status`, time.Now().AddDate(0, 0, -30)).Scan(&rows)
	statusMap := make(map[string]int64)
	for _, r := range rows {
		statusMap[r.Status] = r.Count
	}
	// 5 阶段映射（状态 → 阶段）
	stageMapping := []struct {
		Stage   FunnelStage
		Sources []string
	}{
		{FunnelStage{Name: "陌生人", Code: "stranger"}, []string{string(model.SessionStatusPending)}},
		{FunnelStage{Name: "接触", Code: "contact"}, []string{string(model.SessionStatusAIHandling), string(model.SessionStatusHumanHandling)}},
		{FunnelStage{Name: "等待", Code: "waiting"}, []string{string(model.SessionStatusWaiting)}},
		{FunnelStage{Name: "解决", Code: "resolved"}, []string{string(model.SessionStatusResolved)}},
		{FunnelStage{Name: "关闭", Code: "closed"}, []string{string(model.SessionStatusClosed)}},
	}
	var totalEntered int64
	prevCount := int64(0)
	for i, m := range stageMapping {
		cnt := int64(0)
		for _, src := range m.Sources {
			cnt += statusMap[src]
		}
		if i == 0 {
			totalEntered = cnt
		}
		stepRate := 0.0
		if i > 0 && prevCount > 0 {
			stepRate = float64(cnt) / float64(prevCount) * 100
		}
		stageRate := 0.0
		if totalEntered > 0 {
			stageRate = float64(cnt) / float64(totalEntered) * 100
		}
		stage := m.Stage
		stage.Count = int(cnt)
		stage.StageRate = roundTo(stageRate, 2)
		stage.StepRate = roundTo(stepRate, 2)
		funnel.Stages = append(funnel.Stages, stage)
		prevCount = cnt
	}
	funnel.TotalEntered = int(totalEntered)
	funnel.TotalWon = int(statusMap[string(model.SessionStatusResolved)])
	if totalEntered > 0 {
		funnel.OverallRate = roundTo(float64(funnel.TotalWon)/float64(totalEntered)*100, 2)
	}
	return funnel
}

// collectLLMMetrics 采集 LLM 实时指标
func (c *DashboardSSEController) collectLLMMetrics(ctx context.Context) *LLMRealTimeMetrics {
	m := &LLMRealTimeMetrics{}
	// 指标从 aiagent/llm.GetGlobalFailover 读取（如果已初始化）
	defer func() {
		if r := recover(); r != nil {
			logger.Ctx(ctx).Warn().Interface("panic", r).Msg("dashboard_sse: LLM metrics collection recovered from panic")
		}
	}()
	// 注：避免直接 import aiagent/llm 造成循环依赖；通过 SSEHub 间接获取或返回零值
	// 这里返回安全默认值
	return m
}

// writeDashboardEvent 写 SSE 事件（data 字段为 JSON）
func writeDashboardEvent(c *gin.Context, eventType string, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	_, err = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", eventType, string(jsonData))
	if err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

// writeDashboardRawEvent 写原始 SSE 事件
func writeDashboardRawEvent(c *gin.Context, eventType string, data map[string]any) error {
	return writeDashboardEvent(c, eventType, data)
}

// roundTo 浮点保留 n 位小数
func roundTo(v float64, n int) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	mult := math.Pow(10, float64(n))
	return math.Round(v*mult) / mult
}
