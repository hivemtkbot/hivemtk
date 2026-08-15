package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"hivemtk-user/internal/aiagent/agent/tooluse"
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/pkg/utils/logger"
	feedbackloop "hivemtk-user/internal/service/feedback_loop"
)


// FeedbackCollectorAdapter 适配器
type FeedbackCollectorAdapter struct {
	collector *feedbackloop.FeedbackCollector
	successCount atomic.Int64
	failedCount  atomic.Int64
}

// NewFeedbackCollectorAdapter 创建适配器
//
// 参数：
//   - collector: 全局 FeedbackCollector 单例（nil 时适配器为 no-op）
func NewFeedbackCollectorAdapter(collector *feedbackloop.FeedbackCollector) *FeedbackCollectorAdapter {
	return &FeedbackCollectorAdapter{
		collector: collector,
	}
}

// RecordToolCall 实现 tooluse.FeedbackSink 接口
//
// 将 ToolCallEvent 转换为 CollectRequest 并提交到 FeedbackCollector 异步队列。
// 转换规则：
//   - EventType: implicit（工具调用是隐式反馈信号）
//   - SignalKey: tool_call（统一信号类型，区分工具调用与客户反馈）
//   - SignalValue: 1.0（成功）或 0.0（失败），用于奖励计算
//   - Metadata: 包含 tool_name / args / duration_ms / trace_id / source / risk_level / version
//   - AIReply: 工具参数 JSON（便于后续分析 LLM 决策）
//   - CustomerMsg: 工具结果 JSON（便于后续分析工具输出质量）
func (a *FeedbackCollectorAdapter) RecordToolCall(ctx context.Context, event tooluse.ToolCallEvent) error {
	if a == nil || a.collector == nil {
		return nil
	}

	signalValue := 0.0
	if event.Success {
		signalValue = 1.0
	}

	metadata := map[string]any{
		"tool_name":   event.ToolName,
		"duration_ms": event.Duration.Milliseconds(),
		"trace_id":    event.TraceID,
		"source":      event.Source,
		"success":     event.Success,
	}
	if event.Error != "" {
		metadata["error"] = event.Error
	}
	if event.RiskLevel != "" {
		metadata["risk_level"] = string(event.RiskLevel)
	}
	if event.Version != "" {
		metadata["version"] = event.Version
	}
	if argsJSON, err := marshalArgs(event.Args); err == nil {
		metadata["args"] = argsJSON
	}

	req := &dto.CollectRequest{
		SessionID:   event.SessionID,
		CustomerID:  event.CustomerID,
		EventType:   dto.FBEventTypeImplicit,
		SignalKey:   "tool_call", 
		SignalValue: signalValue,
		AIReply:     "", 
		CustomerMsg: "", 
		Metadata:    metadata,
		CreatedBy:   0, 
	}

	if err := a.collector.Collect(ctx, req); err != nil {
		a.failedCount.Add(1)
		logger.Warnf("[FeedbackSink] collect failed: tool=%s trace=%s err=%v",
			event.ToolName, event.TraceID, err)
		return err
	}
	a.successCount.Add(1)
	return nil
}

// Stats 返回回流统计（用于监控面板）
type FeedbackSinkStats struct {
	SuccessCount int64 `json:"success_count"`
	FailedCount  int64 `json:"failed_count"`
}

// Stats 返回统计信息
func (a *FeedbackCollectorAdapter) Stats() FeedbackSinkStats {
	if a == nil {
		return FeedbackSinkStats{}
	}
	return FeedbackSinkStats{
		SuccessCount: a.successCount.Load(),
		FailedCount:  a.failedCount.Load(),
	}
}

// marshalArgs 序列化参数为 JSON 字符串（限制大小避免 metadata 膨胀）
func marshalArgs(args map[string]any) (string, error) {
	if len(args) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(args)
	if err != nil {
		return "", fmt.Errorf("marshal args: %w", err)
	}
	// 限制最大 4KB（防止超大参数撑爆 metadata 字段）
	const maxArgsSize = 4 * 1024
	if len(b) > maxArgsSize {
		return string(b[:maxArgsSize]) + "...(truncated)", nil
	}
	return string(b), nil
}

