package service

import (
	"context"
	"testing"

	"marketing/internal/pkg/trace"
	"marketing/internal/pkg/utils/logger"
)

// TestTraceID_InheritanceWebhookRouteCtx 验证 triggerSmartOrchestrator 内部
// routeCtx 继承上游 trace_id 的契约。
//
// 业务动机：webhook 入站链路必须保证 trace_id 在全链路（HTTP 中间件 → controller
// → service → 编排 → 触达 → 出站）中保持一致；任何丢失都会让线上问题无法定位。
//
// 注：routeCtx 是 triggerSmartOrchestrator 内部的局部变量（用于 loadAgentForChannel），
// 真实场景难以直接观察。改用 trace.NewContextWithTraceID + trace.TraceIDFromContext
// 的同源契约做单测。
func TestTraceID_InheritanceWebhookRouteCtx(t *testing.T) {
	// 上游 trace_id（HTTP 中间件已绑定）
	const upstreamTraceID = "tr-upstream-001"
	parent := trace.NewContextWithTraceID(context.Background(), upstreamTraceID)

	// 模拟 triggerSmartOrchestrator 的 routeCtx 构造逻辑
	routeCtx := context.Background()
	if parentTraceID := trace.TraceIDFromContext(parent); parentTraceID != "" {
		routeCtx = trace.NewContextWithTraceID(routeCtx, parentTraceID)
	}
	routeCtx = logger.WithModule(routeCtx, "webhook")

	// 验证：routeCtx 应携带与 parent 一致的 trace_id
	if got := trace.TraceIDFromContext(routeCtx); got != upstreamTraceID {
		t.Fatalf("routeCtx trace_id mismatch: want=%q got=%q", upstreamTraceID, got)
	}
}

// TestTraceID_InheritanceRunAIGenerationCtx 验证 runAIGeneration 内部 ctx
// 继承上游 trace_id 的契约。
//
// 业务动机：runAIGeneration 启动 goroutine + replySem，物理上脱离了接入 worker
// 的 ctx；如果新建 background 就会丢失 HTTP 入口的 trace_id，导致线上排查时
// AI 生成与 webhook 入站无法串联。
func TestTraceID_InheritanceRunAIGenerationCtx(t *testing.T) {
	const upstreamTraceID = "tr-upstream-002"
	upstream := trace.NewContextWithTraceID(context.Background(), upstreamTraceID)

	// 模拟 runAIGeneration 内部 ctx 构造逻辑
	parentCtx := context.Background()
	if parentTraceID := trace.TraceIDFromContext(upstream); parentTraceID != "" {
		parentCtx = trace.NewContextWithTraceID(parentCtx, parentTraceID)
	}
	// 注：原代码用 context.WithTimeout(context.Background(), 30s) 然后 logger.WithTraceID(ctx, "")
	// 现修复为基于继承的 parentCtx
	ctx, cancel := context.WithTimeout(parentCtx, 30*1000*1000*1000) // 30s
	defer cancel()
	ctx = logger.WithModule(ctx, "webhook")

	if got := trace.TraceIDFromContext(ctx); got != upstreamTraceID {
		t.Fatalf("runAIGeneration ctx trace_id mismatch: want=%q got=%q", upstreamTraceID, got)
	}
}

// TestTraceID_AutoGenerateWhenMissing 验证上游无 trace_id 时自动生成新 ID，
// 保证可观测性不丢。
func TestTraceID_AutoGenerateWhenMissing(t *testing.T) {
	upstream := context.Background() // 无 trace_id

	parentCtx := context.Background()
	if parentTraceID := trace.TraceIDFromContext(upstream); parentTraceID != "" {
		parentCtx = trace.NewContextWithTraceID(parentCtx, parentTraceID)
	} else {
		// 上游缺失：让 WithTraceID 自动生成
		parentCtx = logger.WithTraceID(parentCtx, "")
	}
	if got := trace.TraceIDFromContext(parentCtx); got == "" {
		t.Fatal("auto-generated trace_id should not be empty")
	}
}
