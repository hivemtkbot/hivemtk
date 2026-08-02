package router

import (
	"context"
	"encoding/json"

	"marketing/internal/aiagent/agent/tooluse"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/service"
)

// ============================================================================
// ToolExecutorAdapter 适配 *tooluse.ToolExecutor → service.AgentToolExecutor
// ----------------------------------------------------------------------------
// 作用：打破 service ↔ tooluse 循环依赖。
//   - service 包定义 AgentToolExecutor 接口（不依赖 tooluse）
//   - tooluse 包的 reach_integration_adapter.go 反向依赖 service（持有 *service.XxxIntegrationService）
//   - 本适配器位于 router 层（依赖 service 和 tooluse 都合法），将两者解耦
//
// 调用方：buildSalesEngine 注入到 SalesEngine.SetToolExecutor
// ============================================================================

// ToolExecutorAdapter 适配器实现
type ToolExecutorAdapter struct {
	executor *tooluse.ToolExecutor
}

// NewToolExecutorAdapter 创建适配器
func NewToolExecutorAdapter(executor *tooluse.ToolExecutor) *ToolExecutorAdapter {
	return &ToolExecutorAdapter{executor: executor}
}

// ListTools 实现 service.AgentToolExecutor.ListTools
// 将 tooluse.LLMFunction 转为 service.AgentToolDef
func (a *ToolExecutorAdapter) ListTools() []service.AgentToolDef {
	if a.executor == nil {
		return nil
	}
	fns := a.executor.ListAvailableLLMFunctions()
	out := make([]service.AgentToolDef, 0, len(fns))
	for _, fn := range fns {
		// 将 ToolParameters 结构体转为 map[string]any（OpenAI JSON Schema 格式）
		paramsMap, err := structToMap(fn.Parameters)
		if err != nil {
			logger.Errorf("[ToolExecutorAdapter] 跳过工具 %s：参数序列化失败 err=%v", fn.Name, err)
			continue
		}
		out = append(out, service.AgentToolDef{
			Name:        fn.Name,
			Description: fn.Description,
			Parameters:  paramsMap,
		})
	}
	return out
}

// DispatchToolCalls 实现 service.AgentToolExecutor.DispatchToolCalls
// 将 service.AgentToolCall 转为 tooluse.LLMToolCall，调用 executor 并发执行，返回结果
func (a *ToolExecutorAdapter) DispatchToolCalls(ctx context.Context, calls []service.AgentToolCall, toolCtx service.AgentToolContext) []service.AgentToolResult {
	if a.executor == nil || len(calls) == 0 {
		return nil
	}
	// 转换 calls
	llmCalls := make([]tooluse.LLMToolCall, 0, len(calls))
	for _, c := range calls {
		llmCalls = append(llmCalls, tooluse.LLMToolCall{
			ID: c.ID,
			Function: tooluse.LLMToolFunction{
				Name:      c.Name,
				Arguments: c.Arguments,
			},
		})
	}
	// 转换 toolCtx
	tooluseCtx := &tooluse.ToolContext{
		AgentID:    toolCtx.AgentID,
		SessionID:  toolCtx.SessionID,
		CustomerID: toolCtx.CustomerID,
		Source:     toolCtx.Source,
	}
	// 执行
	results := a.executor.DispatchByLLMToolCall(ctx, llmCalls, tooluseCtx)
	// 转换结果
	out := make([]service.AgentToolResult, 0, len(results))
	for _, r := range results {
		out = append(out, service.AgentToolResult{
			ToolCallID: r.ToolCallID,
			Content:    r.Content,
			Success:    r.Success,
			Card:       r.Card,
		})
	}
	return out
}

// structToMap 将结构体（或任意值）转为 map[string]any
// 用于将 ToolParameters 序列化为 OpenAI 兼容的 JSON Schema map
func structToMap(v any) (map[string]any, error) {
	if v == nil {
		return map[string]any{"type": "object"}, nil
	}
	bytes, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(bytes, &m); err != nil {
		return nil, err
	}
	return m, nil
}
