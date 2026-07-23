package service

// rag_metrics.go RAG 召回率监控服务（C 域 P1 缺口 #2）
//
// 五层架构归属: L3 业务层
// 设计依据: docs/核心链路优化.md 第十四章 §14.6 召回率监控
//
// 职责:
//   1. RecordQuery(query, retrieved, relevant, latency) - 异步批量写入 rag_query_logs
//   2. GetRecallMetrics(start, end) - 查询时间窗口内的召回指标均值
//   3. GetLowRecallQueries(threshold, limit) - 查询召回率低于阈值的样本（调优用）
//   4. AggregateWindow(windowStart, windowEnd) - 把 rag_query_logs 聚合到 rag_metrics_daily
//   5. 后台 cron 每 5 分钟聚合一次
//
// 私域独立部署: 无 merchant_id 字段

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
)

// ----------------------------------------------------------------------------
// 常量与配置
// ----------------------------------------------------------------------------

const (
	// RagMetricsAggregationInterval 聚合窗口间隔（5 分钟）
	RagMetricsAggregationInterval = 5 * time.Minute
	// RagMetricsBatchSize 异步写入批次大小
	RagMetricsBatchSize = 100
	// RagMetricsFlushInterval 异步刷写间隔
	RagMetricsFlushInterval = 2 * time.Second
	// RagMetricsLowRecallDefault 默认低召回阈值
	RagMetricsLowRecallDefault = 0.3
)

// ----------------------------------------------------------------------------
// 服务结构
// ----------------------------------------------------------------------------

// RagMetricsService RAG 召回率监控服务
type RagMetricsService struct {
	db *gorm.DB

	// 异步批量写入队列
	mu      sync.Mutex
	queue   []*model.RagQueryLog
	flushCh chan struct{}
	stopCh  chan struct{}
	wg      sync.WaitGroup
	started bool
}

// NewRagMetricsService 创建召回率监控服务
//
// db 为 nil 时仅可调用计算类方法（GetRecallMetrics/GetLowRecallQueries），
// 但 RecordQuery 会降级为 no-op
func NewRagMetricsService(db *gorm.DB) *RagMetricsService {
	s := &RagMetricsService{
		db:      db,
		queue:   make([]*model.RagQueryLog, 0, RagMetricsBatchSize),
		flushCh: make(chan struct{}, 1),
		stopCh:  make(chan struct{}),
	}
	return s
}

// Start 启动后台异步写入循环（goroutine）
//
// 必须在主进程启动时调用一次；Stop 时优雅退出
func (s *RagMetricsService) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()

	s.wg.Add(1)
	go s.flushLoop(ctx)
}

// Stop 停止后台 goroutine，刷写剩余日志
func (s *RagMetricsService) Stop(ctx context.Context) {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return
	}
	s.started = false
	s.mu.Unlock()

	close(s.stopCh)
	// 触发最后一次 flush
	select {
	case s.flushCh <- struct{}{}:
	default:
	}
	s.wg.Wait()
	// 兜底再 flush 一次（防止 race）
	_ = s.flush(ctx)
}

// flushLoop 后台定时刷写
func (s *RagMetricsService) flushLoop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(RagMetricsFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			_ = s.flush(ctx)
			return
		case <-ticker.C:
			_ = s.flush(ctx)
		case <-s.flushCh:
			_ = s.flush(ctx)
		}
	}
}

// ----------------------------------------------------------------------------
// RecordQuery 异步记录查询
// ----------------------------------------------------------------------------

// RecordQueryRequest 记录查询请求
type RecordQueryRequest struct {
	Query           string
	SessionID       string
	ProductID       int64
	RetrievedDocIDs []string
	RelevantDocIDs  []string
	Latency         time.Duration
	TopK            int
	Source          string

	// 2026-07-21 新增：Top-1 命中与最高相似度，供 RagRecallMonitor 计算 Top-K / Top-1 命中率与平均相似度
	Top1DocID     string
	HitInTop1     bool
	TopSimilarity float64
}

// RecordQuery 异步记录一次检索（不阻塞调用方）
//
// 计算 precision/recall 并入队；实际写库由后台 goroutine 批量执行
func (s *RagMetricsService) RecordQuery(ctx context.Context, req *RecordQueryRequest) {
	if s == nil || req == nil {
		return
	}
	if s.db == nil {
		return
	}
	log := s.buildQueryLog(ctx, req)
	s.mu.Lock()
	s.queue = append(s.queue, log)
	shouldFlush := len(s.queue) >= RagMetricsBatchSize
	s.mu.Unlock()
	if shouldFlush {
		select {
		case s.flushCh <- struct{}{}:
		default:
		}
	}
}

// RecordQuerySync 同步记录一次检索（测试用，保证写库完成）
func (s *RagMetricsService) RecordQuerySync(ctx context.Context, req *RecordQueryRequest) error {
	if s == nil || req == nil || s.db == nil {
		return fmt.Errorf("service or db is nil")
	}
	log := s.buildQueryLog(ctx, req)
	return s.db.WithContext(ctx).Create(log).Error
}

// buildQueryLog 计算并构造 RagQueryLog
func (s *RagMetricsService) buildQueryLog(ctx context.Context, req *RecordQueryRequest) *model.RagQueryLog {
	retrievedSet := toStringSet(req.RetrievedDocIDs)
	relevantSet := toStringSet(req.RelevantDocIDs)
	hit := 0
	for id := range retrievedSet {
		if _, ok := relevantSet[id]; ok {
			hit++
		}
	}
	precision := 0.0
	if len(retrievedSet) > 0 {
		precision = float64(hit) / float64(len(retrievedSet))
	}
	recall := 0.0
	if len(relevantSet) > 0 {
		recall = float64(hit) / float64(len(relevantSet))
	}
	latencyMs := int64(req.Latency / time.Millisecond)
	topK := req.TopK
	if topK <= 0 {
		topK = 5
	}
	source := req.Source
	if source == "" {
		source = "hybrid"
	}
	return &model.RagQueryLog{
		Query:           req.Query,
		QueryHash:       hashQueryShort(req.Query),
		SessionID:       req.SessionID,
		ProductID:       req.ProductID,
		RetrievedDocIDs: toJSONString(req.RetrievedDocIDs),
		RelevantDocIDs:  toJSONString(req.RelevantDocIDs),
		RetrievedCount:  len(retrievedSet),
		RelevantCount:   len(relevantSet),
		HitCount:        hit,
		Top1DocID:       req.Top1DocID,
		HitInTop1:       req.HitInTop1,
		TopSimilarity:   req.TopSimilarity,
		Precision:       precision,
		Recall:          recall,
		LatencyMs:       latencyMs,
		TopK:            topK,
		Source:          source,
		CreatedAt:       time.Now(),
	}
}

// flush 把队列中的日志批量写入数据库
func (s *RagMetricsService) flush(ctx context.Context) error {
	s.mu.Lock()
	if len(s.queue) == 0 {
		s.mu.Unlock()
		return nil
	}
	batch := s.queue
	s.queue = make([]*model.RagQueryLog, 0, RagMetricsBatchSize)
	s.mu.Unlock()

	if s.db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.db.WithContext(ctx).CreateInBatches(batch, 50).Error; err != nil {
		logger.Errorf("[RagMetrics] flush batch failed (%d logs): %v", len(batch), err)
		return err
	}
	return nil
}

// ----------------------------------------------------------------------------
// GetRecallMetrics 查询时间窗口内的召回指标
// ----------------------------------------------------------------------------

// RecallMetrics 召回指标聚合结果
type RecallMetrics struct {
	WindowStart    time.Time `json:"window_start"`
	WindowEnd      time.Time `json:"window_end"`
	TotalQueries   int64     `json:"total_queries"`
	AvgRecall      float64   `json:"avg_recall"`
	AvgPrecision   float64   `json:"avg_precision"`
	AvgLatencyMs   float64   `json:"avg_latency_ms"`
	P99LatencyMs   int64     `json:"p99_latency_ms"`
	ZeroHitCount   int64     `json:"zero_hit_count"`
	LowRecallCount int64     `json:"low_recall_count"`
}

// GetRecallMetrics 查询时间窗口内的召回指标
func (s *RagMetricsService) GetRecallMetrics(ctx context.Context, start, end time.Time) (*RecallMetrics, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("service or db is nil")
	}
	if end.Before(start) {
		return nil, fmt.Errorf("end before start")
	}

	// 直接从 rag_query_logs 聚合（数据量小用 SQL；数据量大可走 rag_metrics_daily）
	var metrics RecallMetrics
	metrics.WindowStart = start
	metrics.WindowEnd = end

	// 1) 基础聚合：count / avg recall / avg precision / avg latency
	row := struct {
		Total        int64
		AvgRecall    float64
		AvgPrecision float64
		AvgLatency   float64
		ZeroHit      int64
		LowRecall    int64
	}{}
	if err := s.db.WithContext(ctx).
		Model(&model.RagQueryLog{}).
		Where("created_at >= ? AND created_at < ?", start, end).
		Select(`
			COUNT(*) AS total,
			COALESCE(AVG(recall), 0) AS avg_recall,
			COALESCE(AVG(precision), 0) AS avg_precision,
			COALESCE(AVG(latency_ms), 0) AS avg_latency,
			COUNT(*) FILTER (WHERE retrieved_count = 0) AS zero_hit,
			COUNT(*) FILTER (WHERE recall < ?) AS low_recall
		`, RagMetricsLowRecallDefault).
		Scan(&row).Error; err != nil {
		return nil, fmt.Errorf("query recall metrics: %w", err)
	}
	metrics.TotalQueries = row.Total
	metrics.AvgRecall = row.AvgRecall
	metrics.AvgPrecision = row.AvgPrecision
	metrics.AvgLatencyMs = row.AvgLatency
	metrics.ZeroHitCount = row.ZeroHit
	metrics.LowRecallCount = row.LowRecall

	// 2) P99 延迟：取延迟升序排列后第 99 百分位
	// PostgreSQL 没有原生 PERCENTILE_CONT 在所有版本支持，这里用偏移法
	// offset = floor(total * 0.99)，升序排列后跳过 offset 条取下一条
	// 例：10 条数据，offset=9，升序第 10 条 = 最大值
	if row.Total > 0 {
		p99Offset := int(float64(row.Total) * 0.99)
		if p99Offset >= int(row.Total) {
			p99Offset = int(row.Total) - 1
		}
		if p99Offset < 0 {
			p99Offset = 0
		}
		var p99Latency int64
		if err := s.db.WithContext(ctx).
			Model(&model.RagQueryLog{}).
			Where("created_at >= ? AND created_at < ?", start, end).
			Order("latency_ms ASC").
			Offset(p99Offset).
			Limit(1).
			Pluck("latency_ms", &p99Latency).Error; err != nil {
			logger.Errorf("[RagMetrics] query p99 latency failed: %v", err)
		} else {
			metrics.P99LatencyMs = p99Latency
		}
	}

	return &metrics, nil
}

// ----------------------------------------------------------------------------
// GetLowRecallQueries 查询召回率低于阈值的样本
// ----------------------------------------------------------------------------

// LowRecallQuery 低召回样本
type LowRecallQuery struct {
	ID             int64     `json:"id"`
	Query          string    `json:"query"`
	SessionID      string    `json:"session_id"`
	Recall         float64   `json:"recall"`
	Precision      float64   `json:"precision"`
	LatencyMs      int64     `json:"latency_ms"`
	RetrievedCount int       `json:"retrieved_count"`
	RelevantCount  int       `json:"relevant_count"`
	HitCount       int       `json:"hit_count"`
	CreatedAt      time.Time `json:"created_at"`
}

// GetLowRecallQueries 查询召回率低于阈值的样本（用于调优）
//
// threshold ≤ 0 时用默认 0.3
// limit ≤ 0 或 > 1000 时用 100
// 按 created_at DESC 排序（最近的优先）
func (s *RagMetricsService) GetLowRecallQueries(ctx context.Context, threshold float64, limit int) ([]LowRecallQuery, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("service or db is nil")
	}
	if threshold <= 0 {
		threshold = RagMetricsLowRecallDefault
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var rows []LowRecallQuery
	err := s.db.WithContext(ctx).
		Model(&model.RagQueryLog{}).
		Where("recall < ? AND relevant_count > 0", threshold).
		Order("created_at DESC").
		Limit(limit).
		Select("id, query, session_id, recall, precision, latency_ms, retrieved_count, relevant_count, hit_count, created_at").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query low recall: %w", err)
	}
	return rows, nil
}

// ----------------------------------------------------------------------------
// AggregateWindow 聚合窗口数据到 rag_metrics_daily
// ----------------------------------------------------------------------------

// AggregateWindow 把 rag_query_logs 聚合到 rag_metrics_daily
//
// 由 cron 每 5 分钟调用；也可手动调用补跑
// 幂等：同一 window_start 已存在记录则更新
func (s *RagMetricsService) AggregateWindow(ctx context.Context, windowStart, windowEnd time.Time) (*model.RagMetricsDaily, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("service or db is nil")
	}
	metrics, err := s.GetRecallMetrics(ctx, windowStart, windowEnd)
	if err != nil {
		return nil, err
	}
	daily := &model.RagMetricsDaily{
		WindowStart:    windowStart,
		WindowEnd:      windowEnd,
		TotalQueries:   metrics.TotalQueries,
		AvgRecall:      metrics.AvgRecall,
		AvgPrecision:   metrics.AvgPrecision,
		AvgLatencyMs:   metrics.AvgLatencyMs,
		P99LatencyMs:   metrics.P99LatencyMs,
		ZeroHitCount:   metrics.ZeroHitCount,
		LowRecallCount: metrics.LowRecallCount,
		CreatedAt:      time.Now(),
	}
	// 幂等：先查再决定 Create/Update
	var existing model.RagMetricsDaily
	err = s.db.WithContext(ctx).
		Where("window_start = ? AND window_end = ?", windowStart, windowEnd).
		First(&existing).Error
	if err == nil {
		// 更新
		daily.ID = existing.ID
		if err := s.db.WithContext(ctx).Save(daily).Error; err != nil {
			return nil, fmt.Errorf("update rag_metrics_daily: %w", err)
		}
		return daily, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("query existing rag_metrics_daily: %w", err)
	}
	// 新建
	if err := s.db.WithContext(ctx).Create(daily).Error; err != nil {
		return nil, fmt.Errorf("create rag_metrics_daily: %w", err)
	}
	return daily, nil
}

// AggregateLastWindow 聚合最近一个 5 分钟窗口
//
// 用于 cron 调用：windowEnd = now, windowStart = now - 5min
func (s *RagMetricsService) AggregateLastWindow(ctx context.Context) (*model.RagMetricsDaily, error) {
	end := time.Now().Truncate(RagMetricsAggregationInterval)
	start := end.Add(-RagMetricsAggregationInterval)
	return s.AggregateWindow(ctx, start, end)
}

// ----------------------------------------------------------------------------
// GetLatestMetrics 获取最近 N 个聚合窗口
// ----------------------------------------------------------------------------

// GetLatestMetrics 获取最近 N 个聚合窗口（用于趋势图）
func (s *RagMetricsService) GetLatestMetrics(ctx context.Context, limit int) ([]model.RagMetricsDaily, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("service or db is nil")
	}
	if limit <= 0 || limit > 1000 {
		limit = 20
	}
	var rows []model.RagMetricsDaily
	err := s.db.WithContext(ctx).
		Order("window_start DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query latest metrics: %w", err)
	}
	// 按 window_start 升序返回（图表 X 轴顺序）
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].WindowStart.Before(rows[j].WindowStart)
	})
	return rows, nil
}

// ----------------------------------------------------------------------------
// 工具函数
// ----------------------------------------------------------------------------

// toStringSet 把字符串切片转为集合（去重）
func toStringSet(ids []string) map[string]struct{} {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		set[id] = struct{}{}
	}
	return set
}

// toJSONString 把字符串切片序列化为 JSON
func toJSONString(ids []string) string {
	if len(ids) == 0 {
		return "[]"
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// hashQueryShort 计算 query 哈希（短，16 字符）
func hashQueryShort(query string) string {
	h := sha256.Sum256([]byte(query))
	return hex.EncodeToString(h[:])[:16]
}

// ----------------------------------------------------------------------------
// RagMetricsCron 召回指标聚合 cron
// ----------------------------------------------------------------------------

// RagMetricsCron 召回指标聚合定时任务
type RagMetricsCron struct {
	svc    *RagMetricsService
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewRagMetricsCron 创建 cron
func NewRagMetricsCron(svc *RagMetricsService) *RagMetricsCron {
	return &RagMetricsCron{svc: svc, stopCh: make(chan struct{})}
}

// Start 启动 cron
func (c *RagMetricsCron) Start(ctx context.Context) {
	c.wg.Add(1)
	go c.run(ctx)
}

// Stop 停止 cron
func (c *RagMetricsCron) Stop(ctx context.Context) {
	close(c.stopCh)
	c.wg.Wait()
}

// run 定时循环
func (c *RagMetricsCron) run(ctx context.Context) {
	defer c.wg.Done()
	ticker := time.NewTicker(RagMetricsAggregationInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_, err := c.svc.AggregateLastWindow(ctx)
			if err != nil {
				logger.Errorf("[RagMetricsCron] aggregate last window failed: %v", err)
			}
			cancel()
		}
	}
}
