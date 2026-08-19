package service

import (
	"hivemtk-user/internal/service/humanize/behavioral"
)

// BehavioralPlanBuilder 行为层拟人计划构造器
//
// 业界依据：Anthropic 2024 "Building effective agents" + IM UX 研究
//   - 文本层拟人（emoji / 语气词）效果有限（检测器可识别）
//   - 行为层拟人（打字延迟 / 分条发送）真实感更强
//   - WhatsApp / iMessage 等 IM 场景下人类本身就分段发消息
//
// 设计：
//   - 包装 humanize/behavioral 包
//   - 默认禁用（A/B 灰度）；通过 SetBehavioralHumanize(true) 启用
//   - 不影响原有 humanize_polisher 文本层润色（两者独立）
type BehavioralPlanBuilder struct {
	enabled bool
	config  behavioral.BehaviorConfig
}

// NewBehavioralPlanBuilder 构造计划构造器
func NewBehavioralPlanBuilder() *BehavioralPlanBuilder {
	return &BehavioralPlanBuilder{
		enabled: false, // 默认关闭（A/B 灰度）
		config:  behavioral.DefaultBehaviorConfig(),
	}
}

// SetEnabled 设置启用
func (b *BehavioralPlanBuilder) SetEnabled(enabled bool) {
	if b == nil {
		return
	}
	b.enabled = enabled
}

// IsEnabled 返回当前是否启用
func (b *BehavioralPlanBuilder) IsEnabled() bool {
	if b == nil {
		return false
	}
	return b.enabled
}

// SetConfig 设置详细配置
func (b *BehavioralPlanBuilder) SetConfig(cfg behavioral.BehaviorConfig) {
	if b == nil {
		return
	}
	b.config = cfg
}

// Build 为给定文本构造行为层拟人发送计划
//
// inputs:
//   - raw: LLM 原始输出
//   - isFirstMessage: 是否首条消息（影响 thinking pause）
//
// 返回：SendPlan（多条消息 + 间隔 + 总延迟）
//
// 业界依据：
//   - 短文本（< 80 字符）→ 不分条
//   - 长文本 → 按标点分段（业界：句号 > 问号 > 逗号）
//   - 每段间延迟 1.5s ± 20% jitter
//   - 打字延迟：每字符 1/25 秒（人类中位数）
func (b *BehavioralPlanBuilder) Build(raw string, isFirstMessage bool) behavioral.SendPlan {
	if b == nil || !b.enabled {
		// 关闭：返回 trivial plan
		return behavioral.SendPlan{
			Messages:  []string{raw},
			Intervals: nil,
		}
	}
	return behavioral.PlanSend(raw, b.config, isFirstMessage, nil)
}
