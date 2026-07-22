package agent_runtime

import (
	"context"
	"time"
)

// ============================================================================
// Planner Stage 任务规划器阶段
// ----------------------------------------------------------------------------
// 文档依据：方向4 阶段5 任务规划器
//
// 决策依据：
//  1. 意图（决定主工具）
//  2. 情绪（决定回复语气）
//  3. 6维对齐（决定回复风格）
//  4. 知识库挂载（决定是否需要 RAG 检索）
//  5. SOP 挂载（决定是否套用话术）
//
// 输出 ActionPlan：
//  - PlanType   : sales / customer_service / hybrid
//  - ToolCalls  : 计划调用的工具（按优先级排序）
//  - ReplyHint  : 给 LLM 的提示
//  - SkipLLM    : 是否跳过 LLM（标准化 FAQ 直接回复）
// ============================================================================

// DefaultTaskPlanner 默认任务规划器
type DefaultTaskPlanner struct {
	// FAQLibrary 标准化 FAQ 库（可注入）
	// 返回 (matched bool, reply string)
	FAQLibrary func(ctx context.Context, content string) (bool, string)
}

// NewDefaultTaskPlanner 构造默认任务规划器
func NewDefaultTaskPlanner() *DefaultTaskPlanner {
	return &DefaultTaskPlanner{}
}

// NewDefaultTaskPlannerWithFAQ 自定义 FAQ
func NewDefaultTaskPlannerWithFAQ(faq func(ctx context.Context, content string) (bool, string)) *DefaultTaskPlanner {
	return &DefaultTaskPlanner{FAQLibrary: faq}
}

// Name 阶段名
func (p *DefaultTaskPlanner) Name() string {
	return "planner"
}

// Execute 执行规划
func (p *DefaultTaskPlanner) Execute(ctx context.Context, ic *InferenceContext) StageResult {
	start := time.Now()
	plan, err := p.Plan(ctx, ic)
	if err != nil {
		ic.Stages = append(ic.Stages, StageDecision{
			Stage:    p.Name(),
			Action:   "plan_failed",
			Reason:   err.Error(),
			Duration: time.Since(start),
			Success:  false,
			Error:    err.Error(),
		})
		return FailResult(err)
	}
	ic.Plan = plan

	// 写决策
	ic.Decision.Plan = plan
	ic.Decision.Confidence = plan.Confidence
	ic.Decision.StopReason = "plan_ready"

	ic.Stages = append(ic.Stages, StageDecision{
		Stage:    p.Name(),
		Action:   "plan",
		Reason:   "type=" + plan.PlanType + " tools=" + itoa(len(plan.ToolCalls)) + " skip_llm=" + boolStr(plan.SkipLLM),
		Duration: time.Since(start),
		Success:  true,
	})
	return ContinueResult()
}

// Plan 生成行动方案
func (p *DefaultTaskPlanner) Plan(ctx context.Context, ic *InferenceContext) (*ActionPlan, error) {
	plan := &ActionPlan{
		PlanType:   "customer_service",
		Confidence: 0.5,
		ToolCalls:  []PlannedToolCall{},
	}

	// 1. FAQ 优先（如有命中且 SkipLLM=true）
	if p.FAQLibrary != nil {
		if matched, reply := p.FAQLibrary(ctx, ic.Payload.Content); matched {
			plan.SkipLLM = true
			plan.SkipReason = "faq_matched"
			plan.ReplyHint = reply
			plan.Confidence = 0.95
			return plan, nil
		}
	}

	// 2. 根据意图决定工具链
	switch ic.Intent.Primary {
	case IntentGreeting, IntentFarewell, IntentChitchat:
		// 闲聊类：直接 LLM 回复
		plan.PlanType = "customer_service"
		plan.ReplyHint = buildChitchatHint(ic)
		plan.Confidence = 0.85

	case IntentInquiry:
		// 询价：拉知识库 + 查订单
		plan.PlanType = "sales"
		plan.ToolCalls = append(plan.ToolCalls,
			PlannedToolCall{ToolName: "knowledge.search", Args: map[string]any{"query": ic.Payload.Content, "top_k": 3}, Priority: 1},
			PlannedToolCall{ToolName: "product.price", Args: map[string]any{}, Priority: 2},
		)
		plan.ReplyHint = buildSalesHint(ic)
		plan.Confidence = 0.75

	case IntentOrderStatus:
		// 查单：查订单 + 查物流
		plan.PlanType = "customer_service"
		plan.ToolCalls = append(plan.ToolCalls,
			PlannedToolCall{ToolName: "order.lookup", Args: map[string]any{"customer_id": ic.Payload.CustomerID}, Priority: 1},
		)
		plan.ReplyHint = buildOrderHint(ic)
		plan.Confidence = 0.80

	case IntentRefund, IntentComplaint:
		// 退款/投诉：先安抚 + 查订单
		plan.PlanType = "customer_service"
		plan.ToolCalls = append(plan.ToolCalls,
			PlannedToolCall{ToolName: "order.lookup", Args: map[string]any{"customer_id": ic.Payload.CustomerID}, Priority: 1},
		)
		plan.ReplyHint = buildApologizeHint(ic)
		plan.Confidence = 0.70

	case IntentAfterSales:
		plan.PlanType = "customer_service"
		plan.ToolCalls = append(plan.ToolCalls,
			PlannedToolCall{ToolName: "aftersales.policy", Args: map[string]any{}, Priority: 1},
		)
		plan.ReplyHint = buildAfterSalesHint(ic)
		plan.Confidence = 0.70

	case IntentSalesLead:
		plan.PlanType = "sales"
		plan.ToolCalls = append(plan.ToolCalls,
			PlannedToolCall{ToolName: "knowledge.search", Args: map[string]any{"query": ic.Payload.Content, "top_k": 5}, Priority: 1},
		)
		plan.ReplyHint = buildSalesHint(ic)
		plan.Confidence = 0.78

	default:
		// 未知意图：保守回复 + 知识库检索
		plan.PlanType = "customer_service"
		plan.ToolCalls = append(plan.ToolCalls,
			PlannedToolCall{ToolName: "knowledge.search", Args: map[string]any{"query": ic.Payload.Content, "top_k": 3}, Priority: 1},
		)
		plan.ReplyHint = buildDefaultHint(ic)
		plan.Confidence = 0.55
	}

	// 3. 知识库挂载增强（如未挂载则不调用）
	if ic.AgentCtx != nil && !ic.AgentCtx.EnableRAG {
		// 移除所有 knowledge.search 调用
		filtered := []PlannedToolCall{}
		for _, tc := range plan.ToolCalls {
			if tc.ToolName != "knowledge.search" {
				filtered = append(filtered, tc)
			}
		}
		plan.ToolCalls = filtered
	}

	return plan, nil
}

// ============================================================================
// Hint 构造助手
// ============================================================================

func buildChitchatHint(ic *InferenceContext) string {
	persona := ""
	if ic.AgentCtx != nil {
		persona = ic.AgentCtx.Persona
	}
	if persona == "" {
		persona = "友善、专业、温暖"
	}
	return "用" + persona + "的语气回应客户的闲聊。简短、有温度、不啰嗦。"
}

func buildSalesHint(ic *InferenceContext) string {
	hint := "客户正在询价或表达购买意向。请：\n"
	hint += "1. 热情回应（6维-热情度优先）\n"
	hint += "2. 展示产品价值与差异化\n"
	hint += "3. 引导下一步（询单/试用/加购）\n"
	if ic.AgentCtx != nil && ic.AgentCtx.Persona != "" {
		hint += "4. 人设：" + ic.AgentCtx.Persona + "\n"
	}
	return hint
}

func buildOrderHint(ic *InferenceContext) string {
	hint := "客户在查订单。请：\n"
	hint += "1. 调 order.lookup 拿到订单详情\n"
	hint += "2. 用清晰、简洁的格式展示（订单号/商品/状态/预计到达）\n"
	hint += "3. 如有物流单号，附上查询链接\n"
	return hint
}

func buildApologizeHint(ic *InferenceContext) string {
	hint := "客户在投诉/退款。请：\n"
	hint += "1. 先真诚道歉（不要辩解）\n"
	hint += "2. 表达同理心（6维-同理心优先）\n"
	hint += "3. 调 order.lookup 查订单\n"
	hint += "4. 提供明确的下一步（如：1小时内退款/补发/转专员）\n"
	return hint
}

func buildAfterSalesHint(ic *InferenceContext) string {
	return "客户在咨询售后。请：\n1. 调 aftersales.policy 查询售后政策\n2. 给出明确步骤（如何申请/需要哪些凭证/处理时长）\n3. 表达同理心"
}

func buildDefaultHint(ic *InferenceContext) string {
	return "意图不明时：\n1. 礼貌确认客户问题\n2. 调知识库检索（若有）\n3. 如知识库无答案，引导客户换种方式描述"
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
