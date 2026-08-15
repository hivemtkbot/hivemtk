package service

import (
	"context"
	"fmt"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
)


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
		return e.Handle(ctx, req)
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.UserMessage == "" {
		return nil, fmt.Errorf("user_message is empty")
	}

	if resolver := GetAssetBundleResolver(); resolver != nil {
		if persona := resolveAssetBundlePersona(ctx, agentCtx, resolver); persona != "" {
			agentCtx.Persona = persona
		}
	}

	req.Config = dto.AgentContextToSalesEngineConfig(agentCtx)

	req.AgentContext = agentCtx

	return e.Handle(ctx, req)
}


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
	name := c.UnifiedID
	if name == "" {
		name = c.Phone
	}
	if name == "" {
		name = c.Email
	}
	return name
}

