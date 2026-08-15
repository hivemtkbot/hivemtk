// Package agent 是「智能体」领域的总入口。
//
// 设计依据：docs/architecture/adr/-multichannel-cs-sales-requirement-model.md
//
// docs/architecture/adr/-agent-two-mode.md（被动/主动双模式）
//
// 核心理念：
//
//	智能体（AI Agent）= 平台层统一称呼，无论承载「客服」还是「销售」角色，
//	都叫智能体；其角色由 model.AIAgent.AgentType（sales/customer_service/hybrid）
//	区分，对外不再使用"智能体/智能体"。
//
//	智能体真正以 Agent（Tool-Use）形态工作：
//	  - 被动模式(passive)：消息/事件进入系统 → 智能体利用 Tool 工具链（RAG 知识、私信读写、
//	    业务/客户工具）完成对话 → 返回用户。
//	  - 主动模式(active)：智能体主动调用 Tool 工具链触达用户（私信 / 短信 / 邮件 / 卡片），
//	    多用于营销唤起、沉睡唤醒、跟进。
//
// 子包职责（优雅分层）：
//
//	agent/tooluse   - 工具注册中心 + 所有 Tool 实现（customer/reach/private_message/knowledge/business）
//	agent/runtime   - 智能体运行时隔离层（加载上下文、订阅事件、桥接引擎）
//	agent/lifecycle - 双模式生命周期：passive（会话编排）+ active（主动触达引擎）
//	agent/bridge    - 运行时与具体引擎（SalesEngine / SmartCSOrchestrator）的桥接
//
// 渠道维度与能力维度正交：
//
//	渠道（网页客服 / TG / 企微 / 飞书 / 闲鱼 / 抖音 / 快手）只做 Ingest/Send 适配；
//	智能体只理解 AgentContext；二者经 ChannelAgentBinding 解耦。
package agent

import "hivemtk-user/internal/model"

// ModeOf 返回智能体的工作模式（默认被动）。
func ModeOf(a *model.AIAgent) model.AgentMode {
	if a == nil || a.AgentMode == "" {
		return model.AgentModePassive
	}
	return model.AgentMode(a.AgentMode)
}

// IsActive 是否为主动模式。
func IsActive(a *model.AIAgent) bool {
	return ModeOf(a) == model.AgentModeActive
}

