package service

// rag_recall_monitor.go RAG 召回率监控（C 域 P1 缺口 #2 - 监控指标）
//
// 五层架构归属: L3 业务层
// 设计依据: docs/核心链路优化.md 第十四章 §14.6 召回率监控
//
// 与 rag_metrics.go（基础聚合 + 后台异步写入）的关系：
//   - rag_metrics.go        记录每次检索的 precision/recall，写入 rag_query_logs
//   - 本文件                 基于 rag_query_logs 计算专项指标：
//                              * Top-K 命中率  (top_k_hit_rate)
//                              * Top-1 命中率  (top_1_hit_rate)
//                              * 平均相似度     (avg_similarity)
//                              * 检索耗时 P95  (p95_latency_ms)
//                              * 自动定时写入  rag_recall_monitor_snapshots 表
//
// 数据来源：rag_query_logs（每次检索产出一条，由检索服务同步调用 RecordQuery）。
// 采集频率：可配置（默认 5 分钟）
//
// 私域独立部署: 无 merchant_id 字段

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"

	"marketing/internal/repository"
)

// RagRecallMonitorConstants
const (
	// RagRecallMonitorDefaultInterval 默认监控快照采集间隔
	RagRecallMonitorDefaultInterval = 5 * time.Minute
	// RagRecallMonitorDefaultWindow 默认评估窗口（1 小时）
	RagRecallMonitorDefaultWindow = 1 * time.Hour
	// RagRecallMonitorTableName 监控快照表
	RagRecallMonitorTableName = "rag_recall_monitor_snapshots"
)

// RagRecallMetricsSummary 召回率指标汇总
type RagRecallMetricsSummary struct {
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`

	// 基础量
	TotalQueries int64 `json:"total_queries"`

	// 命中指标
	TopKHitRate   float64 `json:"top_k_hit_rate"` // Top-K 命中率（命中数 / 总查询数）
	TopOneHitRate float64 `json:"top_1_hit_rate"` // Top-1 命中率（top-1 命中数 / 总查询数）
	AvgRecall     float64 `json:"avg_recall"`     // 平均召回率
	AvgPrecision  float64 `json:"avg_precision"`  // 平均精确率
	AvgSimilarity float64 `json:"avg_similarity"` // 平均相似度（最高检索分）

	// 性能指标
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	P95LatencyMs int64   `json:"p95_latency_ms"`

	// 分布指标
	ZeroHitCount   int64 `json:"zero_hit_count"`   // 0 命中查询数
	LowRecallCount int64 `json:"low_recall_count"` // 低召回查询数
}

// RagRecallMonitorService RAG 召回率监控服务
type RagRecallMonitorService struct {
	db   *gorm.DB
	repo repository.RagRecallMonitorRepository

	// 异步控制
	mu       sync.Mutex
	started  bool
	stopCh   chan struct{}
	wg       sync.WaitGroup
	interval time.Duration
	window   time.Duration

	// 最后一次快照（用于 API 直接返回，无需走 DB）
	lastSnapshot *RagRecallMetricsSummary
	lastAt       time.Time
}

// NewRagRecallMonitorService 创建 RAG 召回率监控服务
//
// interval / window <= 0 时使用默认值
func NewRagRecallMonitorService(db *gorm.DB, interval, window time.Duration) *RagRecallMonitorService {
	if interval <= 0 {
		interval = RagRecallMonitorDefaultInterval
	}
	if window <= 0 {
		window = RagRecallMonitorDefaultWindow
	}
	return &RagRecallMonitorService{
		db:       db,
		repo:     repository.NewRagRecallMonitorRepository(db),
		stopCh:   make(chan struct{}),
		interval: interval,
		window:   window,
	}
}

// Start 启动后台定时采集
func (s *RagRecallMonitorService) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()

	s.wg.Add(1)
	go s.run(ctx)
}

// Stop 停止后台采集
func (s *RagRecallMonitorService) Stop(ctx context.Context) {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return
	}
	s.started = false
	s.mu.Unlock()

	close(s.stopCh)
	s.wg.Wait()
}

// run 定时循环
func (s *RagRecallMonitorService) run(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if _, err := s.CollectAndStore(ctx, time.Now().Add(-s.window), time.Now()); err != nil {
				cancel()
				continue
			}
			cancel()
		}
	}
}

// Collect 采集指定窗口的指标（不写库）
func (s *RagRecallMonitorService) Collect(ctx context.Context, start, end time.Time) (*RagRecallMetricsSummary, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("service or db is nil")
	}
	if end.Before(start) {
		return nil, errors.New("end before start")
	}

	summary := &RagRecallMetricsSummary{
		WindowStart: start,
		WindowEnd:   end,
	}

	// 1) 基础聚合：count / avg recall / avg precision / avg latency / avg similarity
	row, err := s.repo.AggregateRecallLogs(ctx, start, end)
	if err != nil {
		return nil, fmt.Errorf("query recall aggregate: %w", err)
	}

	summary.TotalQueries = row.Total
	summary.AvgRecall = row.AvgRecall
	summary.AvgPrecision = row.AvgPrecision
	summary.AvgLatencyMs = row.AvgLatency
	summary.AvgSimilarity = row.AvgSimilarity
	summary.ZeroHitCount = row.ZeroHit
	summary.LowRecallCount = row.LowRecall

	if row.Total > 0 {
		summary.TopKHitRate = float64(row.Total-row.ZeroHit) / float64(row.Total)
		summary.TopOneHitRate = float64(row.Top1Hit) / float64(row.Total)
	}

	// 2) P95 延迟
	if row.Total > 0 {
		p95Offset := int(float64(row.Total) * 0.95)
		if p95Offset >= int(row.Total) {
			p95Offset = int(row.Total) - 1
		}
		if p95Offset < 0 {
			p95Offset = 0
		}
		if p95, err := s.repo.PluckP95Latency(ctx, start, end, p95Offset); err == nil {
			summary.P95LatencyMs = p95
		}
	}

	return summary, nil
}

// CollectAndStore 采集指标并写入监控表
func (s *RagRecallMonitorService) CollectAndStore(ctx context.Context, start, end time.Time) (*RagRecallMetricsSummary, error) {
	summary, err := s.Collect(ctx, start, end)
	if err != nil {
		return nil, err
	}

	// 缓存
	s.mu.Lock()
	s.lastSnapshot = summary
	s.lastAt = time.Now()
	s.mu.Unlock()

	// 写入监控快照表
	if s.db != nil {
		payload, _ := json.Marshal(summary)
		row := map[string]any{
			"window_start":     summary.WindowStart,
			"window_end":       summary.WindowEnd,
			"total_queries":    summary.TotalQueries,
			"top_k_hit_rate":   summary.TopKHitRate,
			"top_1_hit_rate":   summary.TopOneHitRate,
			"avg_recall":       summary.AvgRecall,
			"avg_precision":    summary.AvgPrecision,
			"avg_similarity":   summary.AvgSimilarity,
			"avg_latency_ms":   summary.AvgLatencyMs,
			"p95_latency_ms":   summary.P95LatencyMs,
			"zero_hit_count":   summary.ZeroHitCount,
			"low_recall_count": summary.LowRecallCount,
			"payload":          string(payload),
			"created_at":       time.Now(),
		}
		if err := s.repo.CreateSnapshot(ctx, row); err != nil {
			// 写库失败不视为致命错误（日志由调用方处理）
			return summary, fmt.Errorf("store snapshot: %w", err)
		}
	}

	return summary, nil
}

// GetLatestSnapshot 获取最近一次采集的快照（内存中）
func (s *RagRecallMonitorService) GetLatestSnapshot(ctx context.Context) (*RagRecallMetricsSummary, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastSnapshot, s.lastAt
}

// ListSnapshots 列出最近 N 条监控快照
func (s *RagRecallMonitorService) ListSnapshots(ctx context.Context, limit int) ([]map[string]any, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("service or db is nil")
	}
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	rows, err := s.repo.ListSnapshots(ctx, limit)
	if err != nil {
		return nil, err
	}
	// 升序返回（图表 X 轴顺序）
	sort.Slice(rows, func(i, j int) bool {
		ai, _ := rows[i]["window_start"].(time.Time)
		aj, _ := rows[j]["window_start"].(time.Time)
		return ai.Before(aj)
	})
	return rows, nil
}

// EnsureSchema 确保监控表存在（AutoMigrate 风格的轻量 DDL）
//
// 设计：监控表不是核心业务表，使用 CreateTableIfNotExists 模式，
// 部署初始化时调用一次即可。
func (s *RagRecallMonitorService) EnsureSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("service or db is nil")
	}
	return s.repo.EnsureSchema(ctx)
}
