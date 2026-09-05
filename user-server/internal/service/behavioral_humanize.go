package service

import (
	"hivemtk-user/internal/service/humanize/behavioral"
)

// HumanizeScene 拟人场景（H-2：销售场景延迟 ×1.5，客服 2-3s / 销售 5-7s 基线）
type HumanizeScene string

const (
	SceneSupport HumanizeScene = "support"
	SceneSales   HumanizeScene = "sales"
)

const (
	noSplitMaxChars    = 40
	delayBaselineSec   = 2.0
	delayPer20CharsSec = 0.8
	salesSceneFactor   = 1.5
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
	scene   HumanizeScene
}

// NewBehavioralPlanBuilder 构造计划构造器
func NewBehavioralPlanBuilder() *BehavioralPlanBuilder {
	return &BehavioralPlanBuilder{
		enabled: false,
		config:  behavioral.DefaultBehaviorConfig(),
		scene:   SceneSupport,
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

// SetScene 设置场景（影响动态延迟系数），空串视为客服场景
func (b *BehavioralPlanBuilder) SetScene(scene HumanizeScene) {
	if b == nil {
		return
	}
	if scene == "" {
		scene = SceneSupport
	}
	b.scene = scene
}

// SetConfig 设置详细配置
func (b *BehavioralPlanBuilder) SetConfig(cfg behavioral.BehaviorConfig) {
	if b == nil {
		return
	}
	b.config = cfg
}

// Build 为给定文本构造行为层拟人发送计划（兼容入口，使用 builder 当前场景）
func (b *BehavioralPlanBuilder) Build(raw string, isFirstMessage bool) behavioral.SendPlan {
	return b.BuildWithScene(raw, isFirstMessage, b.sceneOrDefault())
}

func (b *BehavioralPlanBuilder) sceneOrDefault() HumanizeScene {
	if b == nil || b.scene == "" {
		return SceneSupport
	}
	return b.scene
}

// BuildWithScene 按指定场景构造发送计划。
//
// H-2 兼容扩展说明：底层 PlanSend 的分条与片段间隔逻辑保留不变，
// 本层做两件事：
//  1. 分条豁免 —— 文本 ≤40 rune 时强制关闭分条（短回复不发多条气泡）
//  2. 动态总延迟 —— 覆盖 PlanSend 的固定打字时间累加：
//     TotalDelay = (基线 2s + ceil(字符数/20)×0.8s) × 场景系数(销售 1.5)
//     非首条消息附加思考停顿 ThinkingPauseSec（沿用原语义）
//
// 业界依据：
//   - 短回复（≤40 字符）禁用分条气泡（HubSpot/Keeper 拟人工实践）
//   - 延迟随长度线性增长模拟真人打字；销售回复更慎重故 ×1.5
func (b *BehavioralPlanBuilder) BuildWithScene(raw string, isFirstMessage bool, scene HumanizeScene) behavioral.SendPlan {
	if b == nil || !b.enabled {

		return behavioral.SendPlan{
			Messages:  []string{raw},
			Intervals: nil,
		}
	}

	cfg := b.config
	n := len([]rune(raw))
	if n <= noSplitMaxChars {
		cfg.EnableMessageSplit = false
	}

	plan := behavioral.PlanSend(raw, cfg, isFirstMessage, nil)
	plan.TotalDelay = dynamicTotalDelay(n, scene, isFirstMessage, cfg)
	return plan
}

func dynamicTotalDelay(n int, scene HumanizeScene, isFirstMessage bool, cfg behavioral.BehaviorConfig) float64 {
	units := (n + 19) / 20
	d := delayBaselineSec + float64(units)*delayPer20CharsSec
	if scene == SceneSales {
		d *= salesSceneFactor
	}
	if !isFirstMessage {
		d += cfg.ThinkingPauseSec
	}
	return d
}
