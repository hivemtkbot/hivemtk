package tooluse

import (
	"context"
	"fmt"
	"sync"
	"time"

	"hivemtk-user/internal/pkg/utils/logger"
)

type AuditLogger interface {
	Log(ctx context.Context, entry AuditEntry)
}

type AuditEntry struct {
	TraceID       string        `json:"trace_id,omitempty"` 
	ToolName      string        `json:"tool_name"`
	CallerID      string        `json:"caller_id"`
	AgentID       string        `json:"agent_id,omitempty"`
	CustomerID    string        `json:"customer_id,omitempty"`
	SessionID     string        `json:"session_id,omitempty"`
	Success       bool          `json:"success"`
	Error         string        `json:"error,omitempty"`
	Duration      time.Duration `json:"duration_ms"`
	RetryCount    int           `json:"retry_count"`
	AuditTrace    string        `json:"audit_trace,omitempty"`
	ArgsSummary   string        `json:"args_summary,omitempty"`   
	ResultSummary string        `json:"result_summary,omitempty"` 
	ExecutedAt    time.Time     `json:"executed_at"`
}

type CostTracker interface {
	Record(ctx context.Context, toolName string, success bool, duration time.Duration) error
}

func AuditDecorator(logger AuditLogger, costTracker CostTracker) ToolDecorator {
	return func(next ToolHandler) ToolHandler {
		return func(ctx context.Context, args map[string]any) (ToolResult, error) {
			toolName := GetToolName(ctx)
			tc := GetToolContext(ctx)
			start := time.Now()

			result, err := next(ctx, args)
			duration := time.Since(start)

			if logger != nil {
				entry := AuditEntry{
					ToolName:   toolName,
					TraceID:    GetTraceID(ctx),
					CallerID:   "",
					AgentID:    "",
					CustomerID: "",
					SessionID:  "",
					Success:    err == nil && result.Success,
					Duration:   duration,
					RetryCount: result.Timing.RetryCount,
					ExecutedAt: start,
				}
				if err != nil {
					entry.Error = err.Error()
				} else if !result.Success {
					entry.Error = result.Error
				}
				if tc != nil {
					entry.CallerID = tc.CallerID
					entry.AgentID = tc.AgentID
					entry.CustomerID = tc.CustomerID
					entry.SessionID = tc.SessionID
					entry.AuditTrace = tc.AuditTrace
				}
				entry.ArgsSummary = summarizeArgs(args)

				entry.ResultSummary = summarizeResult(result.Data)

				logger.Log(ctx, entry)
			}

			if costTracker != nil {
				_ = costTracker.Record(ctx, toolName, err == nil && result.Success, duration)
			}

			recordToolCallMetrics(toolName, err, result, duration)

			return result, err
		}
	}
}

// recordToolCallMetrics 记录工具调用度量
// v3 审计 P0-16 修复：原实现是空函数，所有 4 个参数被 `_ =` 丢弃
// 风险：工具调用的 Prometheus metric（成功率/P99/错误分类）实际从未上报
// 新实现：通过结构化日志埋点，SRE 侧可对接 Loki/Elastic 统计
// 后续如需对接 Prometheus metrics，只需替换 logger.Infof 为 metrics.Counter
func recordToolCallMetrics(toolName string, err error, result ToolResult, duration time.Duration) {
	success := err == nil
	tags := map[string]any{
		"tool":      toolName,
		"success":   success,
		"duration_ms": duration.Milliseconds(),
	}
	if err != nil {
		tags["error"] = err.Error()
	}
	// 结构化日志（zerolog）：后续可被 metrics agent 抓取
	logger.Infof("[tool_metric] %+v", tags)
}

func summarizeArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	sensitiveKeys := map[string]bool{
		"password": true, "token": true, "secret": true,
		"api_key": true, "apikey": true, "phone": true,
		"id_card": true, "bank_card": true,
	}
	out := ""
	for k, v := range args {
		if sensitiveKeys[k] {
			out += fmt.Sprintf("%s=***,", k)
			continue
		}
		s := fmt.Sprintf("%v", v)
		if len(s) > 50 {
			s = s[:50] + "..."
		}
		out += fmt.Sprintf("%s=%s,", k, s)
	}
	if len(out) > 200 {
		out = out[:200] + "..."
	}
	return out
}

func summarizeResult(data any) string {
	if data == nil {
		return ""
	}
	s := fmt.Sprintf("%v", data)
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

type NoOpAuditLogger struct{}

type NoOpCostTracker struct{}

// MemoryAuditLogger 内存审计日志（用于测试 / 单机审计）
type MemoryAuditLogger struct {
	mu      sync.Mutex
	entries []AuditEntry
	maxSize int
}

// NewMemoryAuditLogger 创建内存审计日志
// maxSize: 最大保留条数（超出后滚动覆盖最旧条目）
func NewMemoryAuditLogger(maxSize int) *MemoryAuditLogger {
	if maxSize < 1 {
		maxSize = 1000
	}
	return &MemoryAuditLogger{
		entries: make([]AuditEntry, 0, maxSize),
		maxSize: maxSize,
	}
}

// MemoryCostTracker 内存计费统计
type MemoryCostTracker struct {
	mu      sync.Mutex
	records map[string]*costRecord
}

type costRecord struct {
	TotalCalls      int64
	SuccessCalls    int64
	FailedCalls     int64
	TotalDurationMs int64
}

// NewMemoryCostTracker 创建内存计费统计
func NewMemoryCostTracker() *MemoryCostTracker {
	return &MemoryCostTracker{
		records: make(map[string]*costRecord),
	}
}

// CostStats 计费统计快照
type CostStats struct {
	ToolName        string  `json:"tool_name"`
	TotalCalls      int64   `json:"total_calls"`
	SuccessCalls    int64   `json:"success_calls"`
	FailedCalls     int64   `json:"failed_calls"`
	TotalDurationMs int64   `json:"total_duration_ms"`
	SuccessRate     float64 `json:"success_rate"`
	AvgDurationMs   float64 `json:"avg_duration_ms"`
}

