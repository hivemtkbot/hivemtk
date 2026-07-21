// Package agent_bridge 提供 agent_runtime 与 service 包之间的桥接器实现。
//
// 设计目的：打破 import cycle
//   - agent_runtime 包定义接口(SalesEngineBridge / SmartCSBridge),不依赖 service
//   - service 包依赖 agent_runtime(单向)
//   - agent_bridge 包同时依赖 agent_runtime 和 service,实现接口
//
// 部署方式：在 main.go 中:
//
//	bridge := agent_bridge.NewSalesEngineBridge(salesEngine)
//	rt := agent_runtime.NewAgentRuntime(loader, bridge, csBridge)
package agent_bridge

import (
	"context"
	"time"

	"marketing/internal/aiagent/agent/runtime"
	"marketing/internal/dto"
	"marketing/internal/service"
)

// ============================================================================
// salesEngineBridge — 销售引擎桥接器
// ============================================================================

// salesEngineBridge 销售引擎桥接器
type salesEngineBridge struct {
	engine *service.SalesEngine
}

// NewSalesEngineBridge 创建销售引擎桥接器
func NewSalesEngineBridge(engine *service.SalesEngine) agent_runtime.SalesEngineBridge {
	return &salesEngineBridge{engine: engine}
}

// HandleWithAgent 调销售引擎
func (b *salesEngineBridge) HandleWithAgent(ctx context.Context, agentCtx *agent_runtime.AgentContext, req *agent_runtime.SalesRequest) (*agent_runtime.SalesResponse, error) {
	if b.engine == nil {
		return nil, agent_runtime.ErrBridgeNotInitialized
	}

	start := time.Now()

	// 1. 类型转换:agent_runtime → dto
	dtoAgent := convertToDTOAgentContext(agentCtx)
	dtoReq := convertToDTOSalesRequest(req, dtoAgent)

	// 2. 调引擎
	resp, err := b.engine.HandleWithAgent(ctx, dtoReq, dtoAgent)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}

	// 3. 响应转换
	return convertFromDTOSalesResponse(resp, agentCtx, req, time.Since(start)), nil
}

// ============================================================================
// 转换函数
// ============================================================================

// convertToDTOAgentContext agent_runtime.AgentContext → dto.AgentContext
func convertToDTOAgentContext(ac *agent_runtime.AgentContext) *dto.AgentContext {
	if ac == nil {
		return nil
	}
	return &dto.AgentContext{
		AgentID:              ac.AgentID,
		AgentCode:            ac.AgentCode,
		Name:                 ac.Name,
		AgentType:            ac.AgentType,
		Persona:              ac.Persona,
		SystemPrompt:         ac.SystemPrompt,
		Greeting:             ac.Greeting,
		RagProductIDs:        ac.RagProductIDs,
		SOPIDs:               ac.SOPIDs,
		ScriptLibraryIDs:     ac.ScriptLibraryIDs,
		DecisionStrategyIDs:  ac.DecisionStrategyIDs,
		ABExperimentIDs:      ac.ABExperimentIDs,
		LLMModel:             ac.LLMModel,
		Temperature:          ac.Temperature,
		MaxTokens:            ac.MaxTokens,
		TopP:                 ac.TopP,
		EnableRAG:            ac.EnableRAG,
		EnableScriptMatch:    ac.EnableScriptMatch,
		EnableHumanizePolish: ac.EnableHumanizePolish,
		EnableContentAudit:   ac.EnableContentAudit,
		EnablePlaybook:       ac.EnablePlaybook,
		RAGTopK:              ac.RAGTopK,
		ConfidenceThreshold:  ac.ConfidenceThreshold,
		MaxAIConsecutive:     ac.MaxAIConsecutive,
		Version:              ac.Version,
	}
}

// convertToDTOSalesRequest agent_runtime.SalesRequest → dto.SalesRequest
func convertToDTOSalesRequest(req *agent_runtime.SalesRequest, agentCtx *dto.AgentContext) *dto.SalesRequest {
	if req == nil {
		return nil
	}
	return &dto.SalesRequest{
		CustomerID:   req.CustomerID,
		UserMessage:  req.Content,
		Platform:     req.Channel,
		AutoExecute:  true, // agent_runtime 路径默认 AI 接管
		AgentContext: agentCtx,
	}
}

// convertFromDTOSalesResponse dto.SalesResponse → agent_runtime.SalesResponse
func convertFromDTOSalesResponse(resp *dto.SalesResponse, ac *agent_runtime.AgentContext, req *agent_runtime.SalesRequest, duration time.Duration) *agent_runtime.SalesResponse {
	if resp == nil {
		return nil
	}

	// 提取工具调用链
	toolsCalled := make([]string, 0)
	if resp.MatchedSOP != nil {
		toolsCalled = append(toolsCalled, "sop:"+resp.MatchedSOP.Scenario)
	}
	if resp.ScriptTemplate != nil {
		toolsCalled = append(toolsCalled, "script:"+resp.ScriptTemplate.ID)
	}
	if len(resp.RAGChunks) > 0 {
		toolsCalled = append(toolsCalled, "rag")
	}

	confidence := 0.0
	if resp.Intent != nil {
		confidence = resp.Intent.Confidence
	}

	return &agent_runtime.SalesResponse{
		ReplyContent:   resp.Reply,
		ReplyType:      "text",
		Confidence:     confidence,
		AgentID:        ac.AgentID,
		AgentCode:      ac.AgentCode,
		Channel:        req.Channel,
		CustomerID:     req.CustomerID,
		TraceID:        req.TraceID,
		ToolsCalled:    toolsCalled,
		LLMModel:       resp.LLMModel,
		TokensUsed:     resp.CostTokens,
		HandoffToHuman: resp.TransferredToHuman,
		StopReason:     resp.TransferReason,
		Duration:       duration,
	}
}
