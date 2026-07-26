package agent_runtime

import (
	"context"
	"strings"
	"time"
)

// ============================================================================
// Gatekeeper Stage 危机感安全门禁阶段
// ----------------------------------------------------------------------------
// 文档依据：方向4 阶段4 危机感安全门禁
//
// 判定逻辑（按文档）：
//  1. 危机感评分 >= 4 分 → 强制转人工
//  2. 检测到"退款/骗子/起诉"等关键词 → 转人工
//  3. 否则继续走任务规划器
//
// 危机等级：
//  - High   (3) - 强制转人工（命中熔断）
//  - Medium (2) - 标记为需要关注（不强制转人工，但记入观测）
//  - Low    (1) - 观察
//  - None   (0) - 无危机
// ============================================================================

// DefaultCrisisDetector 默认危机感检测器
type DefaultCrisisDetector struct {
	// HighRiskKeywords 高危关键词（命中即 CrisisHigh）
	HighRiskKeywords []string
	// MediumRiskKeywords 中危关键词
	MediumRiskKeywords []string
	// LowRiskKeywords 低危关键词
	LowRiskKeywords []string
}

// NewDefaultCrisisDetector 构造默认危机检测器
func NewDefaultCrisisDetector() *DefaultCrisisDetector {
	return &DefaultCrisisDetector{
		HighRiskKeywords: []string{
			"退款", "退货", "骗子", "骗人", "欺诈", "起诉", "曝光", "315",
			"工商", "消协", "黑猫", "12315", "律师", "报警",
			"refund", "scam", "fraud", "lawsuit", "sue", "lawyer", "police",
		},
		MediumRiskKeywords: []string{
			"投诉", "差评", "气死", "生气", "愤怒", "无语", "失望",
			"complain", "angry", "furious", "disappointed",
		},
		LowRiskKeywords: []string{
			"急", "着急", "焦虑", "担心", "麻烦",
			"urgent", "worried", "anxious",
		},
	}
}

// Name 阶段名
func (d *DefaultCrisisDetector) Name() string {
	return "gatekeeper"
}

// Execute 执行门禁
func (d *DefaultCrisisDetector) Execute(ctx context.Context, ic *InferenceContext) StageResult {
	start := time.Now()
	ic.Crisis = d.Detect(ctx, ic)

	// 决策记录
	action := "pass"
	if ic.Crisis.NeedsEscalation() {
		action = "escalate"
	} else if ic.Crisis.Level >= CrisisMedium {
		action = "flag"
	}

	ic.Stages = append(ic.Stages, StageDecision{
		Stage:    d.Name(),
		Action:   action,
		Reason:   "level=" + itoa(int(ic.Crisis.Level)) + " triggers=" + strings.Join(ic.Crisis.Triggers, ","),
		Duration: time.Since(start),
		Success:  true,
	})

	// 危机等级 High → 立即返回转人工决策
	if ic.Crisis.NeedsEscalation() {
		ic.Decision.HandoffToHuman = true
		ic.Decision.HandoffReason = ic.Crisis.Reason
		ic.Decision.StopReason = "crisis_gate_triggered"
		ic.Decision.Reply = "已为您转接人工客服，请稍等～"
		ic.Decision.ReplyType = "handoff"
		return StopResult(&ic.Decision)
	}

	return ContinueResult()
}

// Detect 检测危机等级
func (d *DefaultCrisisDetector) Detect(ctx context.Context, ic *InferenceContext) CrisisSignal {
	text := strings.ToLower(ic.Payload.Content)
	triggers := []string{}
	level := CrisisNone
	reason := "no_crisis"

	// 0. 强转人工意图（方向6 文档示例4：用户明确要求"找人工" → 直接 CrisisHigh）
	if ic.Intent.Primary == IntentHandoffToHuman {
		level = CrisisHigh
		reason = "handoff_human_intent"
		triggers = append(triggers, "intent:handoff_human")
	}

	// 1. 高危关键词扫描
	for _, kw := range d.HighRiskKeywords {
		if strings.Contains(text, strings.ToLower(kw)) {
			triggers = append(triggers, kw)
			level = CrisisHigh
			reason = "high_risk_keyword:" + kw
		}
	}

	// 2. 中危关键词（不覆盖高危）
	if level < CrisisMedium {
		for _, kw := range d.MediumRiskKeywords {
			if strings.Contains(text, strings.ToLower(kw)) {
				triggers = append(triggers, kw)
				if level < CrisisMedium {
					level = CrisisMedium
					reason = "medium_risk_keyword:" + kw
				}
			}
		}
	}

	// 3. 低危关键词
	if level == CrisisNone {
		for _, kw := range d.LowRiskKeywords {
			if strings.Contains(text, strings.ToLower(kw)) {
				triggers = append(triggers, kw)
				level = CrisisLow
				reason = "low_risk_keyword:" + kw
			}
		}
	}

	// 4. 情绪加权：愤怒 + 强度高 → 升级到 Medium
	if level < CrisisMedium && ic.Sentiment.Label == SentimentAngry && ic.Sentiment.Score >= 0.7 {
		level = CrisisMedium
		reason = "angry_sentiment_high"
	}

	// 5. 对齐分数加权：6维中同理心低 → 升级
	if level < CrisisMedium && ic.Alignment.Empathy <= 2 {
		level = CrisisMedium
		reason = "low_empathy_alignment"
	}

	// 6. 强转人工意图再次提升（与关键词合并时升级到 High）
	if level < CrisisHigh && ic.Intent.Primary == IntentHandoffToHuman {
		level = CrisisHigh
		reason = "handoff_human_intent_promoted"
	}

	return CrisisSignal{
		Level:      level,
		Triggers:   triggers,
		Reason:     reason,
		DetectedAt: time.Now(),
	}
}
