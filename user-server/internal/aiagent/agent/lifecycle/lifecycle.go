// Package lifecycle 定义智能体的「双模式」生命周期。
//
// 设计依据：docs/architecture/adr/-agent-two-mode.md
//
// 两种模式共享同一套 Tool 工具链与 AgentContext，差异仅在"谁触发"：
//
//	┌─────────────────────────────────────────────────────────────┐
//	│ 被动模式 PassiveAgentLifecycle                                │
//	│   触发：渠道消息/事件进入系统（用户主动发消息 / 系统事件）      │
//	│   流程：Ingest → MessageHub → 解析智能体 → Tool 链(RAG/私信/业务)│
//	│         → 生成回复 → Send 回原渠道                              │
//	│   归属：智能体 / 智能体应答主路径（对话域）                   │
//	└─────────────────────────────────────────────────────────────┘
//
//	┌─────────────────────────────────────────────────────────────┐
//	│ 主动模式 ActiveAgentLifecycle                                 │
//	│   触发：定时任务 / 营销事件 / 运营策略                          │
//	│   流程：策略选材 → 智能体决策 → Tool 链(私信/短信/邮件/卡片)     │
//	│         → 触达用户 → 召回进入被动会话 / 归因 OneID              │
//	│   归属：营销唤起、沉睡唤醒、主动跟进（更多由人工策略编排）       │
//	└─────────────────────────────────────────────────────────────┘
//
// 说明：本文件为双模式领域骨架，确立命名与接口边界；具体编排逻辑复用
// agent/runtime 与 service.SmartCSOrchestrator，新增主动触达引擎在后续阶段落地。
package lifecycle

import (
	"context"

	"hivemtk-user/internal/dto"
)

// AgentLifecycle 智能体生命周期统一接口。
// 被动/主动两种模式都实现本接口，由上层根据 model.AgentMode 选择。
type AgentLifecycle interface {
	// Mode 返回本生命周期的工作模式。
	Mode() string
	// Run 在给定智能体上下文下执行一轮（被动=应答；主动=触达）。
	Run(ctx context.Context, agentCtx *dto.AgentContext, req *LifecycleRequest) (*LifecycleResult, error)
}

// LifecycleRequest 生命周期请求（被动/主动通用）。
type LifecycleRequest struct {
	Channel    string         // 渠道
	AccountID  string         // 渠道账号
	CustomerID string         // 客户 OneID（主动模式可空，由策略选材）
	Content    string         // 输入内容（被动=用户消息；主动=策略/提示）
	TraceID    string         // 全链路追踪
	Raw        map[string]any // 原始字段
}

// LifecycleResult 生命周期结果。
type LifecycleResult struct {
	ReplyContent string   // 回复/触达内容
	ToolsCalled  []string // 本次调用过的工具名
	Handoff      bool     // 是否转人工
	StopReason   string   // 终止原因
}

// Resolver 按智能体模式选择生命周期实现。
// 被动模式返回 Passive；主动模式返回 Active；未知回退 Passive。
func Resolver(passive, active AgentLifecycle) func(mode string) AgentLifecycle {
	return func(mode string) AgentLifecycle {
		if mode == "active" && active != nil {
			return active
		}
		return passive
	}
}
