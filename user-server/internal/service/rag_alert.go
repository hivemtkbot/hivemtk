package service

// rag_alert.go RAG 风控预警服务（C 域 P1 缺口 #3）
//
// 五层架构归属: L3 业务层
// 设计依据: docs/核心链路优化.md 第十四章 §14.6.3 风控预警
//
// 职责:
//   1. CheckAndAlert(windowStart, windowEnd) - 检查时间窗口内的指标，触发预警
//   2. GetActiveAlerts(alertType, limit) - 查询活跃预警（未解决）
//   3. ResolveAlert(id, resolvedBy, note) - 标记预警已解决
//   4. 后台 cron 每 5 分钟检查一次
//
// 预警条件（4 种）:
//   a) low_recall       - 召回率 5 分钟均值 < 0.3
//   b) embedding_failure - 向量化失败率 > 10%
//   c) high_latency     - 检索 P99 延迟 > 2 秒
//   d) zero_hit         - 知识库空命中（0 docs）占比 > 20%
//
// 严重度分级:
//   - message: 信息（首次出现）
//   - warning: 警告（持续 3 个窗口）
//   - critical: 严重（持续 6 个窗口或单次极端值）
//
// 幂等性: 同一窗口同一类型已存在活跃预警时不重复创建
//
// 私域独立部署: 无 merchant_id 字段

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	kbmodel "marketing/internal/aiagent/knowledge/model"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
)

// ----------------------------------------------------------------------------
// 常量与配置
// ----------------------------------------------------------------------------

const (
	// RagAlertCheckInterval cron 检查间隔（5 分钟）
	RagAlertCheckInterval = 5 * time.Minute

	// RagAlertLowRecallThreshold 召回率阈值（低于触发预警）
	RagAlertLowRecallThreshold = 0.3
	// RagAlertEmbeddingFailureThreshold 向量化失败率阈值（高于触发预警）
	RagAlertEmbeddingFailureThreshold = 0.10
	// RagAlertHighLatencyThreshold P99 延迟阈值（毫秒，高于触发预警）
	RagAlertHighLatencyThreshold = 2000
	// RagAlertZeroHitRatioThreshold 空命中占比阈值（高于触发预警）
	RagAlertZeroHitRatioThreshold = 0.20

	// RagAlertWarningPersistWindows warning 级别所需的持续窗口数
	RagAlertWarningPersistWindows = 3
	// RagAlertCriticalPersistWindows critical 级别所需的持续窗口数
	RagAlertCriticalPersistWindows = 6

	// RagAlertDefaultLimit 默认查询条数
	RagAlertDefaultLimit = 100
)

// ----------------------------------------------------------------------------
// 服务结构
// ----------------------------------------------------------------------------

// RagAlertService RAG 风控预警服务
type RagAlertService struct {
	db     *gorm.DB
	metric *RagMetricsService

	// 用于 cron 控制
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewRagAlertService 创建风控预警服务
//
// db: 主数据库（用于查询 rag_alerts / rag_query_logs / knowledge_documents）
// metric: 召回率监控服务（用于查询窗口指标）；为 nil 时内部新建一个
func NewRagAlertService(db *gorm.DB, metric *RagMetricsService) *RagAlertService {
	if metric == nil {
		metric = NewRagMetricsService(db)
	}
	return &RagAlertService{
		db:     db,
		metric: metric,
		stopCh: make(chan struct{}),
	}
}

// ----------------------------------------------------------------------------
// AlertCheckResult 检查结果
// ----------------------------------------------------------------------------

// AlertCheckResult 单次检查结果（用于测试和 API 返回）
type AlertCheckResult struct {
	WindowStart       time.Time         `json:"window_start"`
	WindowEnd         time.Time         `json:"window_end"`
	TriggeredAlerts   []*model.RagAlert `json:"triggered_alerts"`
	SkippedDuplicates int               `json:"skipped_duplicates"`
	Checks            []AlertCheckItem  `json:"checks"`
}

// AlertCheckItem 单项检查详情
type AlertCheckItem struct {
	Type        string  `json:"type"`
	MetricValue float64 `json:"metric_value"`
	Threshold   float64 `json:"threshold"`
	Triggered   bool    `json:"triggered"`
	Reason      string  `json:"reason"`
}

// ----------------------------------------------------------------------------
// CheckAndAlert 主入口：检查窗口内的指标并触发预警
// ----------------------------------------------------------------------------

// CheckAndAlert 检查指定时间窗口的指标，按需创建预警
//
// 行为：
//  1. 查询 rag_query_logs 窗口聚合（recall/precision/p99/zero_hit）
//  2. 查询 knowledge_documents 向量化失败率
//  3. 对每项指标判断是否触发阈值
//  4. 触发时检查是否已有同窗口同类型活跃预警（幂等）
//  5. 根据持续窗口数确定严重度
func (s *RagAlertService) CheckAndAlert(ctx context.Context, windowStart, windowEnd time.Time) (*AlertCheckResult, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("service or db is nil")
	}
	if windowEnd.Before(windowStart) {
		return nil, fmt.Errorf("window_end before window_start")
	}

	result := &AlertCheckResult{
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	}

	// 1) 查询召回率指标
	recallMetrics, err := s.metric.GetRecallMetrics(ctx, windowStart, windowEnd)
	if err != nil {
		return nil, fmt.Errorf("query recall metrics: %w", err)
	}

	// 2) 查询向量化失败率
	embedFailRate, embedTotal, err := s.getEmbeddingFailureRate(ctx)
	if err != nil {
		logger.Errorf("[RagAlert] query embedding failure rate failed: %v", err)
		embedFailRate = 0
	}

	// 3) 计算空命中占比
	zeroHitRatio := 0.0
	if recallMetrics.TotalQueries > 0 {
		zeroHitRatio = float64(recallMetrics.ZeroHitCount) / float64(recallMetrics.TotalQueries)
	}

	// 4) 逐项检查
	checks := []AlertCheckItem{
		{
			Type:        string(model.RagAlertTypeLowRecall),
			MetricValue: recallMetrics.AvgRecall,
			Threshold:   RagAlertLowRecallThreshold,
			Triggered:   recallMetrics.TotalQueries > 0 && recallMetrics.AvgRecall < RagAlertLowRecallThreshold,
			Reason:      fmt.Sprintf("avg_recall=%.4f < threshold=%.4f", recallMetrics.AvgRecall, RagAlertLowRecallThreshold),
		},
		{
			Type:        string(model.RagAlertTypeEmbeddingFailure),
			MetricValue: embedFailRate,
			Threshold:   RagAlertEmbeddingFailureThreshold,
			Triggered:   embedTotal > 0 && embedFailRate > RagAlertEmbeddingFailureThreshold,
			Reason:      fmt.Sprintf("embedding_fail_rate=%.4f > threshold=%.4f (total=%d)", embedFailRate, RagAlertEmbeddingFailureThreshold, embedTotal),
		},
		{
			Type:        string(model.RagAlertTypeHighLatency),
			MetricValue: float64(recallMetrics.P99LatencyMs),
			Threshold:   float64(RagAlertHighLatencyThreshold),
			Triggered:   recallMetrics.TotalQueries > 0 && recallMetrics.P99LatencyMs > int64(RagAlertHighLatencyThreshold),
			Reason:      fmt.Sprintf("p99_latency=%dms > threshold=%dms", recallMetrics.P99LatencyMs, RagAlertHighLatencyThreshold),
		},
		{
			Type:        string(model.RagAlertTypeZeroHit),
			MetricValue: zeroHitRatio,
			Threshold:   RagAlertZeroHitRatioThreshold,
			Triggered:   recallMetrics.TotalQueries > 0 && zeroHitRatio > RagAlertZeroHitRatioThreshold,
			Reason:      fmt.Sprintf("zero_hit_ratio=%.4f > threshold=%.4f (zero=%d/total=%d)", zeroHitRatio, RagAlertZeroHitRatioThreshold, recallMetrics.ZeroHitCount, recallMetrics.TotalQueries),
		},
	}
	result.Checks = checks

	// 5) 对触发的检查创建预警
	for _, check := range checks {
		if !check.Triggered {
			continue
		}
		// 幂等：检查同窗口同类型是否已有活跃预警
		exists, err := s.hasActiveAlert(ctx, check.Type, windowStart, windowEnd)
		if err != nil {
			logger.Errorf("[RagAlert] check existing alert failed (%s): %v", check.Type, err)
			continue
		}
		if exists {
			result.SkippedDuplicates++
			continue
		}

		// 根据持续窗口数确定严重度
		severity := s.determineSeverity(ctx, check.Type, windowStart)

		alert := &model.RagAlert{
			AlertType:   check.Type,
			Severity:    string(severity),
			MetricValue: check.MetricValue,
			Threshold:   check.Threshold,
			Message:     check.Reason,
			WindowStart: windowStart,
			WindowEnd:   windowEnd,
			Resolved:    false,
			CreatedAt:   time.Now(),
		}
		if err := s.db.WithContext(ctx).Create(alert).Error; err != nil {
			logger.Errorf("[RagAlert] create alert failed (%s): %v", check.Type, err)
			continue
		}
		result.TriggeredAlerts = append(result.TriggeredAlerts, alert)
	}

	return result, nil
}

// CheckLastWindow 检查最近一个 5 分钟窗口
func (s *RagAlertService) CheckLastWindow(ctx context.Context) (*AlertCheckResult, error) {
	end := time.Now().Truncate(RagAlertCheckInterval)
	start := end.Add(-RagAlertCheckInterval)
	return s.CheckAndAlert(ctx, start, end)
}

// ----------------------------------------------------------------------------
// GetActiveAlerts 查询活跃预警
// ----------------------------------------------------------------------------

// GetActiveAlerts 查询活跃（未解决）预警
//
// alertType 为空时查询所有类型；按 created_at DESC 排序
func (s *RagAlertService) GetActiveAlerts(ctx context.Context, alertType string, limit int) ([]model.RagAlert, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("service or db is nil")
	}
	if limit <= 0 || limit > 1000 {
		limit = RagAlertDefaultLimit
	}
	q := s.db.WithContext(ctx).
		Where("resolved = false").
		Order("created_at DESC").
		Limit(limit)
	if alertType != "" {
		q = q.Where("alert_type = ?", alertType)
	}
	var alerts []model.RagAlert
	if err := q.Find(&alerts).Error; err != nil {
		return nil, fmt.Errorf("query active alerts: %w", err)
	}
	return alerts, nil
}

// GetAlertHistory 查询预警历史（含已解决）
func (s *RagAlertService) GetAlertHistory(ctx context.Context, alertType string, limit int) ([]model.RagAlert, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("service or db is nil")
	}
	if limit <= 0 || limit > 1000 {
		limit = RagAlertDefaultLimit
	}
	q := s.db.WithContext(ctx).
		Order("created_at DESC").
		Limit(limit)
	if alertType != "" {
		q = q.Where("alert_type = ?", alertType)
	}
	var alerts []model.RagAlert
	if err := q.Find(&alerts).Error; err != nil {
		return nil, fmt.Errorf("query alert history: %w", err)
	}
	return alerts, nil
}

// ----------------------------------------------------------------------------
// ResolveAlert 解决预警
// ----------------------------------------------------------------------------

// ResolveAlert 标记预警为已解决
//
// 同一类型同窗口的活跃预警会被批量解决（避免遗漏）
func (s *RagAlertService) ResolveAlert(ctx context.Context, id int64, resolvedBy, note string) (*model.RagAlert, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("service or db is nil")
	}
	if id <= 0 {
		return nil, fmt.Errorf("invalid alert id")
	}
	var alert model.RagAlert
	if err := s.db.WithContext(ctx).First(&alert, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("alert not found: %d", id)
		}
		return nil, fmt.Errorf("query alert: %w", err)
	}
	if alert.Resolved {
		return &alert, nil // 已解决，幂等返回
	}
	now := time.Now()
	alert.Resolved = true
	alert.ResolvedAt = &now
	alert.ResolvedBy = resolvedBy
	alert.ResolveNote = note
	if err := s.db.WithContext(ctx).Save(&alert).Error; err != nil {
		return nil, fmt.Errorf("update alert: %w", err)
	}
	return &alert, nil
}

// ResolveAllActive 解决指定类型的所有活跃预警
func (s *RagAlertService) ResolveAllActive(ctx context.Context, alertType, resolvedBy, note string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("service or db is nil")
	}
	now := time.Now()
	q := s.db.WithContext(ctx).
		Model(&model.RagAlert{}).
		Where("resolved = false")
	if alertType != "" {
		q = q.Where("alert_type = ?", alertType)
	}
	res := q.Updates(map[string]any{
		"resolved":     true,
		"resolved_at":  now,
		"resolved_by":  resolvedBy,
		"resolve_note": note,
	})
	if err := res.Error; err != nil {
		return 0, fmt.Errorf("resolve alerts: %w", err)
	}
	return res.RowsAffected, nil
}

// ----------------------------------------------------------------------------
// 内部辅助方法
// ----------------------------------------------------------------------------

// hasActiveAlert 检查同窗口同类型是否已有活跃预警
func (s *RagAlertService) hasActiveAlert(ctx context.Context, alertType string, windowStart, windowEnd time.Time) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).
		Model(&model.RagAlert{}).
		Where("alert_type = ? AND window_start = ? AND window_end = ? AND resolved = false",
			alertType, windowStart, windowEnd).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// determineSeverity 根据持续窗口数确定严重度
//
// 逻辑：
//   - 当前窗口往前数 6 个窗口（不含当前），若都触发过同类型预警 → critical
//   - 否则往前数 3 个窗口（不含当前），若都触发过 → warning
//   - 否则 → message
//
// 注意：此处用已存在的预警记录近似"持续窗口数"
func (s *RagAlertService) determineSeverity(ctx context.Context, alertType string, currentWindowStart time.Time) model.RagAlertSeverity {
	// 查询过去 6 个窗口（30 分钟）内的同类型预警数（含已解决的，作为"曾经触发"的依据）
	since := currentWindowStart.Add(-time.Duration(RagAlertCriticalPersistWindows) * RagAlertCheckInterval)
	var count int64
	if err := s.db.WithContext(ctx).
		Model(&model.RagAlert{}).
		Where("alert_type = ? AND window_start >= ? AND window_start < ?",
			alertType, since, currentWindowStart).
		Count(&count).Error; err != nil {
		logger.Errorf("[RagAlert] query history count failed (%s): %v", alertType, err)
		return model.RagAlertSeverityMessage
	}
	if count >= int64(RagAlertCriticalPersistWindows) {
		return model.RagAlertSeverityCritical
	}
	if count >= int64(RagAlertWarningPersistWindows) {
		return model.RagAlertSeverityWarning
	}
	return model.RagAlertSeverityMessage
}

// getEmbeddingFailureRate 查询向量化失败率
//
// 查询条件：knowledge_documents 全量
// 失败率 = embed_status='failed' 的数量 / 总数
func (s *RagAlertService) getEmbeddingFailureRate(ctx context.Context) (rate float64, total int64, err error) {
	var stats struct {
		Total  int64
		Failed int64
	}
	if err = s.db.WithContext(ctx).
		Model(&kbmodel.KnowledgeDocument{}).
		Select(`
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE embed_status = 'failed') AS failed
		`).
		Scan(&stats).Error; err != nil {
		return 0, 0, err
	}
	if stats.Total == 0 {
		return 0, 0, nil
	}
	return float64(stats.Failed) / float64(stats.Total), stats.Total, nil
}

// ----------------------------------------------------------------------------
// RagAlertCron 风控预警定时任务
// ----------------------------------------------------------------------------

// RagAlertCron 风控预警 cron
type RagAlertCron struct {
	svc    *RagAlertService
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewRagAlertCron 创建 cron
func NewRagAlertCron(svc *RagAlertService) *RagAlertCron {
	return &RagAlertCron{svc: svc, stopCh: make(chan struct{})}
}

// Start 启动 cron
func (c *RagAlertCron) Start(ctx context.Context)  {
	c.wg.Add(1)
	go c.run(ctx)
}

// Stop 停止 cron
func (c *RagAlertCron) Stop(ctx context.Context)  {
	close(c.stopCh)
	c.wg.Wait()
}

// run 定时循环
func (c *RagAlertCron) run(ctx context.Context)  {
	defer c.wg.Done()
	ticker := time.NewTicker(RagAlertCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if _, err := c.svc.CheckLastWindow(ctx); err != nil {
				logger.Errorf("[RagAlertCron] check last window failed: %v", err)
			}
			cancel()
		}
	}
}

// ----------------------------------------------------------------------------
// 工具函数
// ----------------------------------------------------------------------------

// FormatAlertSummary 格式化预警摘要（用于日志/通知）
func FormatAlertSummary(alerts []model.RagAlert) string {
	if len(alerts) == 0 {
		return "无活跃预警"
	}
	var b strings.Builder
	for i, a := range alerts {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(fmt.Sprintf("[%s][%s] %s (value=%.4f, threshold=%.4f)",
			a.Severity, a.AlertType, a.Message, a.MetricValue, a.Threshold))
	}
	return b.String()
}
