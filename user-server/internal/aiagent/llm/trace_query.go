package llm

import (
	"context"
	"errors"
	"sort"

	"hivemtk-user/internal/pkg/db"
	"gorm.io/gorm"
)

// TraceQueryError 查询失败错误（含 reason 便于上层判定）。
type TraceQueryError struct {
	Reason string
	Err    error
}

func (e *TraceQueryError) Error() string {
	if e.Err != nil {
		return e.Reason + ": " + e.Err.Error()
	}
	return e.Reason
}

func (e *TraceQueryError) Unwrap() error { return e.Err }

// ErrTraceNotFound trace_id 在 trace_events 表无任何记录。
var ErrTraceNotFound = errors.New("trace not found")

// TraceSummary 一次 trace 的概要统计。
type TraceSummary struct {
	TraceID     string `json:"trace_id"`
	SpanCount   int    `json:"span_count"`
	KindCounts  map[string]int `json:"kind_counts"`
	ServiceCount int   `json:"service_count"`
	FirstAt     string `json:"first_at,omitempty"`
	LastAt      string `json:"last_at,omitempty"`
	TotalDurationMs int64 `json:"total_duration_ms"`
	ErrorCount  int    `json:"error_count"`
}

// TraceSpanView 单条 span 的视图（不暴露内部元数据 raw 文本）。
type TraceSpanView struct {
	SpanID       string         `json:"span_id"`
	ParentSpanID string         `json:"parent_span_id,omitempty"`
	Kind         string         `json:"kind"`
	Service      string         `json:"service"`
	Operation    string         `json:"operation"`
	DurationMs   int64          `json:"duration_ms"`
	Status       string         `json:"status"`
	Timestamp    string         `json:"timestamp"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// TraceDetail 单条 trace 的完整视图（按时间排序的 spans + 概要）。
type TraceDetail struct {
	Summary TraceSummary    `json:"summary"`
	Spans   []TraceSpanView `json:"spans"`
}

// traceDBOverride 仅用于单元测试注入 DB（默认 nil → 走 db.GetDB()）。
// 不导出，避免外部包误用；测试通过 t.Cleanup 还原。
var traceDBOverride func() any

// fetchDB 取 DB 实例：测试 override 优先；否则走全局 db.GetDB()。
func fetchDB() *gorm.DB {
	if traceDBOverride != nil {
		if v, ok := traceDBOverride().(*gorm.DB); ok {
			return v
		}
		return nil
	}
	return db.GetDB()
}

// QueryTrace 按 trace_id 查整条链路明细。
//
//   - 返回 spans 按 timestamp ASC 排序，方便前端顺序展示。
//   - summary 含 kind 分布 / service 数量 / 首个-末个 span 时间 / 错误数 / 累计耗时。
//   - 找不到返回 (nil, ErrTraceNotFound)，由 controller 翻译为 404。
func QueryTrace(ctx context.Context, traceID string) (*TraceDetail, error) {
	if traceID == "" {
		return nil, &TraceQueryError{Reason: "trace_id is empty"}
	}
	d := fetchDB()
	if d == nil {
		return nil, &TraceQueryError{Reason: "db not initialized"}
	}

	rows, err := fetchTraceRows(ctx, d, traceID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrTraceNotFound
	}

	detail := &TraceDetail{
		Spans:  make([]TraceSpanView, 0, len(rows)),
		Summary: TraceSummary{
			TraceID:     traceID,
			SpanCount:   len(rows),
			KindCounts:  make(map[string]int, 4),
			ServiceCount: 0,
		},
	}
	services := make(map[string]struct{}, 4)
	for _, r := range rows {
		ts := ""
		if t, ok := r["timestamp"].(interface{ String() string }); ok {
			ts = t.String()
		}
		detail.Spans = append(detail.Spans, TraceSpanView{
			SpanID:       stringOf(r["span_id"]),
			ParentSpanID: stringOf(r["parent_span_id"]),
			Kind:         stringOf(r["kind"]),
			Service:      stringOf(r["service"]),
			Operation:    stringOf(r["operation"]),
			DurationMs:   int64Of(r["duration_ms"]),
			Status:       stringOf(r["status"]),
			Timestamp:    ts,
			Metadata:     UnmarshalMetadata(stringOf(r["metadata"])),
		})
		detail.Summary.KindCounts[stringOf(r["kind"])]++
		if _, ok := services[stringOf(r["service"])]; !ok {
			services[stringOf(r["service"])] = struct{}{}
		}
		if stringOf(r["status"]) != "ok" {
			detail.Summary.ErrorCount++
		}
		detail.Summary.TotalDurationMs += int64Of(r["duration_ms"])
	}
	detail.Summary.ServiceCount = len(services)

	if len(detail.Spans) > 0 {
		detail.Summary.FirstAt = detail.Spans[0].Timestamp
		detail.Summary.LastAt = detail.Spans[len(detail.Spans)-1].Timestamp
	}

	sort.SliceStable(detail.Spans, func(i, j int) bool {
		return detail.Spans[i].Timestamp < detail.Spans[j].Timestamp
	})
	return detail, nil
}

// ListRecentTraces 返回最近 N 个 trace_id 概要（按 last span 时间倒序）。
//
// 用途：监控面板/调试 UI 列最近活跃 trace，定位现场用。
// limit<=0 或 >500 自动归一为 50。
func ListRecentTraces(ctx context.Context, limit int) ([]TraceSummary, error) {
	d := fetchDB()
	if d == nil {
		return nil, &TraceQueryError{Reason: "db not initialized"}
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	type traceRow struct {
		TraceID  string
		LastAt   string
		SpanCount int
	}
	var ids []traceRow
	if err := d.WithContext(ctx).
		Table("trace_events").
		Select("trace_id, MAX(timestamp) as last_at, COUNT(*) as span_count").
		Group("trace_id").
		Order("MAX(timestamp) DESC").
		Limit(limit).
		Scan(&ids).Error; err != nil {
		return nil, &TraceQueryError{Reason: "list trace_ids", Err: err}
	}
	if len(ids) == 0 {
		return []TraceSummary{}, nil
	}

	out := make([]TraceSummary, 0, len(ids))
	// 一次查询补齐 kind_counts / service_count（v3 审计：避免前端看到 null）
	traceIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		traceIDs = append(traceIDs, id.TraceID)
	}
	statsMap := fetchTraceSummaryStats(d, traceIDs)

	for _, id := range ids {
		stats := statsMap[id.TraceID]
		out = append(out, TraceSummary{
			TraceID:      id.TraceID,
			SpanCount:    id.SpanCount,
			KindCounts:   stats.kindCounts,
			ServiceCount: stats.serviceCount,
			FirstAt:      id.LastAt,
			LastAt:       id.LastAt,
		})
	}
	return out, nil
}

// traceSummaryStats 一次查询补齐 kind_counts / service_count
type traceSummaryStats struct {
	kindCounts   map[string]int
	serviceCount int
}

// fetchTraceSummaryStats 批量查询避免 N+1
func fetchTraceSummaryStats(d *gorm.DB, traceIDs []string) map[string]traceSummaryStats {
	if len(traceIDs) == 0 {
		return map[string]traceSummaryStats{}
	}
	// 业界优化：单次 GROUP BY 查询替代 N 次
	var rows []struct {
		TraceID string `gorm:"column:trace_id"`
		Kind    string `gorm:"column:kind"`
		Service string `gorm:"column:service"`
	}
	if err := d.Table("trace_events").
		Select("trace_id, kind, service").
		Where("trace_id IN ?", traceIDs).
		Group("trace_id, kind, service").
		Scan(&rows).Error; err != nil {
		// 容错：失败时返回空 map，前端用 null 显示
		return map[string]traceSummaryStats{}
	}
	result := make(map[string]traceSummaryStats, len(traceIDs))
	for _, r := range rows {
		stats, ok := result[r.TraceID]
		if !ok {
			stats = traceSummaryStats{kindCounts: make(map[string]int, 4)}
		}
		stats.kindCounts[r.Kind]++
		stats.serviceCount++
		result[r.TraceID] = stats
	}
	return result
}

// fetchTraceRows 从 trace_events 表按 trace_id 拉所有 span（按 timestamp ASC）。
// 返回 raw map（避免依赖 llm.TraceEvent 模型 GORM 反射）。
func fetchTraceRows(ctx context.Context, d *gorm.DB, traceID string) ([]map[string]any, error) {
	var rows []map[string]any
	if err := d.WithContext(ctx).
		Table("trace_events").
		Where("trace_id = ?", traceID).
		Order("timestamp ASC, id ASC").
		Scan(&rows).Error; err != nil {
		return nil, &TraceQueryError{Reason: "fetch trace_events", Err: err}
	}
	return rows, nil
}

func stringOf(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func int64Of(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}
