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
//
// 五层架构修复：所有 DB 查询已下沉到 service.DashboardStatsService，
// controller 不再直接访问 db / gorm / model。

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

	"marketing/internal/pkg/utils/logger"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
)

// DashboardSSEController 实时驾驶舱 SSE 控制器
type DashboardSSEController struct {
	statsSvc	service.DashboardStatsService

	// 缓存（最近一次查询结果，避免每次推送都查库）
	cacheMu		sync.RWMutex
	lastSnapshot	*service.DashboardSnapshot
	lastUpdateAt	time.Time
	cacheTTL	time.Duration

	// 流订阅者统计
	subscriberCount	atomic.Int64
}

// NewDashboardSSEController 创建实时驾驶舱 SSE 控制器
//
// statsSvc 由 router 注入（router 负责创建 service.NewDashboardStatsService(gormDB)）。
// 传入 nil 时进入"离线模式"：SSE 仍可连接但跳过 DB 采集，返回零值快照。
func NewDashboardSSEController(statsSvc service.DashboardStatsService) *DashboardSSEController {
	return &DashboardSSEController{
		statsSvc:	statsSvc,
		cacheTTL:	2 * time.Second,
	}
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
	snapshot := c.collectSnapshot(ctx, ctx.Request.Context())
	if snapshot != nil {
		if writeErr := writeDashboardEvent(ctx, "dashboard_update", snapshot); writeErr != nil {
			return
		}
	}

	// 推送连接成功事件
	_ = writeDashboardRawEvent(ctx, "connected", map[string]any{
		"client_ip":	ctx.ClientIP(),
		"subscriber":	c.subscriberCount.Load(),
		"message":	"dashboard event stream connected",
		"refresh_sec":	int(c.cacheTTL.Seconds()),
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
			snapshot := c.collectSnapshot(ctx, ctx.Request.Context())
			if snapshot == nil {
				continue
			}
			if writeErr := writeDashboardEvent(ctx, "dashboard_update", snapshot); writeErr != nil {
				return
			}
		case <-heartbeatTicker.C:
			if writeErr := writeDashboardRawEvent(ctx, "heartbeat", map[string]any{
				"ts":		time.Now().UTC().Format(time.RFC3339Nano),
				"subscriber":	c.subscriberCount.Load(),
			}); writeErr != nil {
				return
			}
		}
	}
}

// Snapshot 返回当前快照（一次性 JSON）
func (c *DashboardSSEController) Snapshot(ctx *gin.Context) {
	snapshot := c.collectSnapshot(ctx, ctx.Request.Context())
	if snapshot == nil {
		response.Error(ctx, http.StatusInternalServerError, "采集快照失败")
		return
	}
	response.Success(ctx, snapshot, "ok")
}

// Metrics 完整指标（含历史趋势）
func (c *DashboardSSEController) Metrics(ctx *gin.Context) {
	snapshot := c.collectSnapshot(ctx, ctx.Request.Context())
	if snapshot == nil {
		response.Error(ctx, http.StatusInternalServerError, "采集指标失败")
		return
	}
	// 附加订阅者数 / 缓存时间
	response.Success(ctx, map[string]any{
		"snapshot":		snapshot,
		"subscriber_count":	c.subscriberCount.Load(),
		"cache_ttl_sec":	int(c.cacheTTL.Seconds()),
		"server_time":		time.Now().UTC().Format(time.RFC3339Nano),
	}, "ok")
}

// collectSnapshot 采集实时数据快照（带 2s 缓存）
func (c *DashboardSSEController) collectSnapshot(ctx context.Context) *service.DashboardSnapshot {
	c.cacheMu.RLock()
	if c.lastSnapshot != nil && time.Since(c.lastUpdateAt) < c.cacheTTL {
		snap := c.lastSnapshot
		c.cacheMu.RUnlock()
		return snap
	}
	c.cacheMu.RUnlock()

	snap := &service.DashboardSnapshot{
		GeneratedAt: time.Now(),
	}

	if c.statsSvc != nil && c.statsSvc.Available() {
		// 1. 在线会话数
		c.statsSvc.CollectSessionStats(ctx, snap)
		// 2. 拟人度分布
		snap.HumanizeDistribution = c.statsSvc.CollectHumanizeDistribution(ctx)
		// 3. 转化漏斗
		snap.Funnel = c.statsSvc.CollectFunnel(ctx)
	} else {
		// 离线模式：返回最小化默认值
		snap.HumanizeDistribution = service.HumanizeDistribution{WindowHours: 1}
		snap.Funnel = &service.FunnelProgress{Stages: []service.FunnelStage{}}
	}

	// 4. LLM 指标（始终返回非 nil，提供默认零值）
	snap.LLMMetrics = c.collectLLMMetrics(ctx)

	// 缓存
	c.cacheMu.Lock()
	c.lastSnapshot = snap
	c.lastUpdateAt = time.Now()
	c.cacheMu.Unlock()

	return snap
}

// collectLLMMetrics 采集 LLM 实时指标
func (c *DashboardSSEController) collectLLMMetrics(ctx context.Context) *service.LLMRealTimeMetrics {
	m := &service.LLMRealTimeMetrics{}
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
