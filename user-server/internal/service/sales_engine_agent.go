package service

import (
	"context"
	"fmt"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
)

// ============================================================================
// SalesEngine 多智能体扩展方法
// ----------------------------------------------------------------------------
// 设计文档：docs/sales-champion/MULTI_AI_AGENT_DESIGN.md
//
// 本文件为 SalesEngine 增加按智能体上下文执行的能力，保持向后兼容：
//   - 原 Handle(ctx, req) 不变
//   - 新增 HandleWithAgent(ctx, req, agentCtx)
//
// 当 agentCtx == nil 时回退到原 Handle 流程
// ============================================================================

// HandleWithAgent 按指定智能体上下文执行 9 步链路
//
// 调用方：
//   - WebhookService.triggerSalesEngine：渠道账号收到消息后按绑定智能体执行
//   - SmartCSOrchestrator.HandleIncomingWithAgent：客服座席按挂载智能体执行
//   - AIAgentService.TestAgent：智能体测试执行
//
// 改造策略：
//  1. agentCtx == nil → 回退到原 Handle
//  2. agentCtx != nil → 用 agentCtx 覆盖 req.Config，注入 Persona/RAG/SOP 等
//  3. 将 agentCtx 注入 SalesRequest.AgentContext 字段（新增字段，向下兼容）
//
// 注意：RAG 知识库路由、SOP 过滤在 recallRAG / matchSOP 内部判断 agentCtx
func (e *SalesEngine) HandleWithAgent(ctx context.Context, req *SalesRequest, agentCtx *AgentContext) (*SalesResponse, error) {
	if agentCtx == nil {
		// 回退默认流程
		return e.Handle(ctx, req)
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.UserMessage == "" {
		return nil, fmt.Errorf("user_message is empty")
	}

	// 0. 资产包人设织入（智能体→资产包，覆盖原 Persona）
	// 任何故障都会安全降级（resolveAssetBundlePersona 内部已处理 resolver==nil / 解析失败）
	if resolver := GetAssetBundleResolver(); resolver != nil {
		if persona := resolveAssetBundlePersona(ctx, agentCtx, resolver); persona != "" {
			agentCtx.Persona = persona
		}
	}

	// 1. 用 agentCtx 覆盖 req.Config
	req.Config = dto.AgentContextToSalesEngineConfig(agentCtx)

	// 2. 注入 agentCtx 到 SalesRequest（供 recallRAG / matchSOP 使用）
	req.AgentContext = agentCtx

	// 3. 调用原 Handle 流程（内部 9 步链路会读取 req.AgentContext）
	return e.Handle(ctx, req)
}

// HasAgentContext / RagProductIDsForRequest / SOPIDsForRequest / AgentPersonaForRequest / AgentLLMModelForRequest
// 5 个方法已迁移至 dto/sales.go（深度 DTO 迁移-5）
// type alias 不允许定义新方法，故不在 service 包保留重复定义

// CustomerFromAgent 提供给 LLM 上下文的客户标识信息
// 优先级：UnifiedID > Phone > Email
// 选择理由：Customer 模型没有 Name/OneID 字段，使用 UnifiedID 作为统一标识最为稳定；
// 缺失时依次回退到 Phone / Email，保证总能返回非空标识（除非三者均为空）。
//
// 扩展点：若未来需要按 agentCtx.AgentType 决定客户信息呈现方式（例如 B2B 场景
// 输出公司名 + 职位，B2C 场景输出昵称），可在此处增加分支逻辑。
func CustomerFromAgent(c *model.Customer, agentCtx *AgentContext) string {
	if c == nil {
		return ""
	}
	// Customer 模型没有 Name/OneID 字段，使用 UnifiedID/Phone/Email 作为回退标识
	name := c.UnifiedID
	if name == "" {
		name = c.Phone
	}
	if name == "" {
		name = c.Email
	}
	return name
}
