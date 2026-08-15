package agent_runtime

import (
	"context"
	"strings"
	"time"
)


// DefaultCrisisDetector 默认危机感检测器
type DefaultCrisisDetector struct {
	HighRiskKeywords []string
	MediumRiskKeywords []string
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

	if ic.Intent.Primary == IntentHandoffToHuman {
		level = CrisisHigh
		reason = "handoff_human_intent"
		triggers = append(triggers, "intent:handoff_human")
	}

	for _, kw := range d.HighRiskKeywords {
		if strings.Contains(text, strings.ToLower(kw)) {
			triggers = append(triggers, kw)
			level = CrisisHigh
			reason = "high_risk_keyword:" + kw
		}
	}

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

	if level == CrisisNone {
		for _, kw := range d.LowRiskKeywords {
			if strings.Contains(text, strings.ToLower(kw)) {
				triggers = append(triggers, kw)
				level = CrisisLow
				reason = "low_risk_keyword:" + kw
			}
		}
	}

	if level < CrisisMedium && ic.Sentiment.Label == SentimentAngry && ic.Sentiment.Score >= 0.7 {
		level = CrisisMedium
		reason = "angry_sentiment_high"
	}

	if level < CrisisMedium && ic.Alignment.Empathy <= 2 {
		level = CrisisMedium
		reason = "low_empathy_alignment"
	}

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

