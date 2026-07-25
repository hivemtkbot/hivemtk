package agent_runtime

import "marketing/internal/pkg/i18n"

// InternalPromptTemplates 内部工具 prompt 多语言版本
//
// 每个工具至少提供 zh + en 两个版本，按 internal_language 渲染。
// 当前内部工具（planner_stage.go 等）仍使用内置中文 prompt，本表作为未来
// 多语言改造的扩展点：当 internal_language != "zh" 时，可切换到本表对应版本。
//
// 模板 key：
//   - sop_match        : SOP 流程匹配
//   - objection_handle : 异议处理
//   - planner          : 任务规划
var InternalPromptTemplates = map[string]map[string]string{
	"sop_match": {
		"zh": "你是一名电商客服专家。请根据用户意图与情绪，从候选 SOP 流程中匹配最佳流程，并输出流程编号与匹配理由。\n要求：\n1. 仅基于用户当前问题与上下文进行匹配\n2. 若无匹配流程，明确返回【无匹配】\n3. 输出格式：流程编号 | 置信度(0-1) | 匹配理由",
		"en": "You are an e-commerce customer service expert. Based on the customer's intent and sentiment, match the best SOP workflow from the candidates and output the workflow ID with the matching rationale.\nRequirements:\n1. Match only on the customer's current question and context\n2. If no workflow matches, explicitly return [no match]\n3. Output format: workflow_id | confidence(0-1) | rationale",
	},
	"objection_handle": {
		"zh": "你是一名电商客服专家，负责处理客户异议。请识别异议类型（价格/质量/物流/服务/信任），并给出针对性回应策略。\n要求：\n1. 共情客户顾虑，避免对抗\n2. 提供事实依据或替代方案\n3. 保持专业、友好语气",
		"en": "You are an e-commerce customer service expert handling customer objections. Identify the objection type (price/quality/logistics/service/trust) and provide a targeted response strategy.\nRequirements:\n1. Empathize with the customer's concern; avoid confrontation\n2. Provide factual evidence or alternatives\n3. Maintain a professional, friendly tone",
	},
	"planner": {
		"zh": "你是一名电商客服规划师。请根据用户意图、情绪与知识库检索结果，规划回复策略与工具调用顺序。\n要求：\n1. 询价类：引导下单，必要时调知识库\n2. 查单类：调订单查询工具，给出明确状态\n3. 投诉/退款类：调售后政策，表达同理心并给步骤\n4. 售后咨询类：调 aftersales.policy，给凭证与时长\n5. 意图不明：礼貌确认问题，必要时知识库检索",
		"en": "You are an e-commerce customer service planner. Based on the customer's intent, sentiment, and knowledge base retrieval results, plan the reply strategy and tool call order.\nRequirements:\n1. Pricing inquiries: guide toward purchase; query knowledge base if needed\n2. Order tracking: call the order query tool and give a clear status\n3. Complaints/refunds: call after-sales policy; empathize and provide steps\n4. After-sales consultation: call aftersales.policy; state required proof and SLA\n5. Unclear intent: politely confirm the question; query knowledge base if needed",
	},
}

// RenderInternalPrompt 按 internal_language 渲染内部工具 prompt
//
// internalLang 为小写短码（如 "zh"/"en"）。未命中时兜底中文；若模板 key 不存在返回空串。
// 注意：当前内部工具暂未强制使用本函数，保留作为未来多语言改造的接入点。
func RenderInternalPrompt(templateKey, internalLang string) string {
	templates, ok := InternalPromptTemplates[templateKey]
	if !ok {
		return ""
	}
	if tpl, ok := templates[internalLang]; ok && tpl != "" {
		return tpl
	}
	// 兜底中文
	if tpl, ok := templates[string(i18n.ZH)]; ok {
		return tpl
	}
	return ""
}
