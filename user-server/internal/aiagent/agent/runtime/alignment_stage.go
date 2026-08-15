package agent_runtime

import (
	"context"
	"time"
)


// DefaultAlignmentScorer 默认 6 维拟人度评分器
type DefaultAlignmentScorer struct {
	RAGConfidenceFunc func(ctx context.Context, agentCtx *AgentContext, content string) float64
	TurnCountFunc func(ctx context.Context, agentCtx *AgentContext, customerID string) int
}

// NewDefaultAlignmentScorer 构造默认评分器
func NewDefaultAlignmentScorer() *DefaultAlignmentScorer {
	return &DefaultAlignmentScorer{}
}

// NewDefaultAlignmentScorerWith 自定义评分器
func NewDefaultAlignmentScorerWith(
	rag func(ctx context.Context, agentCtx *AgentContext, content string) float64,
	turn func(ctx context.Context, agentCtx *AgentContext, customerID string) int,
) *DefaultAlignmentScorer {
	return &DefaultAlignmentScorer{
		RAGConfidenceFunc: rag,
		TurnCountFunc:     turn,
	}
}

// Name 阶段名
func (s *DefaultAlignmentScorer) Name() string {
	return "alignment"
}

// Execute 执行打分
func (s *DefaultAlignmentScorer) Execute(ctx context.Context, ic *InferenceContext) StageResult {
	start := time.Now()
	ic.Alignment = s.Score(ctx, ic)

	ic.Stages = append(ic.Stages, StageDecision{
		Stage:    s.Name(),
		Action:   "score",
		Reason:   "total=" + formatFloat(ic.Alignment.Total()) + " weakest=" + string(ic.Alignment.MaxDimension()),
		Duration: time.Since(start),
		Success:  true,
	})
	return ContinueResult()
}

// Score 计算 6 维评分
func (s *DefaultAlignmentScorer) Score(ctx context.Context, ic *InferenceContext) AlignmentScore {
	score := AlignmentScore{
		Empathy:    3,
		Enthusiasm: 3,
		Expertise:  3,
		Patience:   3,
		Clarity:    3,
		Politeness: 3,
	}

	switch ic.Sentiment.Label {
	case SentimentAngry:
		score.Empathy = 5
	case SentimentAnxious:
		score.Empathy = 4
	case SentimentConfused:
		score.Empathy = 4
	case SentimentAppreci:
		score.Empathy = 4
	case SentimentCalm:
		score.Empathy = 3
	}

	switch ic.Intent.Primary {
	case IntentInquiry, IntentSalesLead:
		score.Enthusiasm = 5
	case IntentAppreciIntent():
		score.Enthusiasm = 5
	case IntentOrderStatus, IntentAfterSales:
		score.Enthusiasm = 4
	case IntentComplaint, IntentRefund:
		score.Enthusiasm = 3
	case IntentGreeting, IntentChitchat:
		score.Enthusiasm = 4
	default:
		score.Enthusiasm = 3
	}

	if s.RAGConfidenceFunc != nil && ic.AgentCtx != nil {
		ragScore := s.RAGConfidenceFunc(ctx, ic.AgentCtx, ic.Payload.Content)
		score.Expertise = clamp(int(ragScore*4)+1, 1, 5)
	}

	if s.TurnCountFunc != nil && ic.AgentCtx != nil {
		turns := s.TurnCountFunc(ctx, ic.AgentCtx, ic.Payload.CustomerID)
		switch {
		case turns >= 10:
			score.Patience = 5
		case turns >= 5:
			score.Patience = 4
		case turns >= 1:
			score.Patience = 3
		default:
			score.Patience = 3
		}
	}

	contentLen := len(ic.Payload.Content)
	switch {
	case contentLen >= 10 && contentLen <= 200:
		score.Clarity = 4
	case contentLen < 10:
		score.Clarity = 3 
	default:
		score.Clarity = 3 
	}

	politenessMarkers := []string{"请", "谢谢", "麻烦", "您好", "请帮我", "please", "thanks"}
	for _, m := range politenessMarkers {
		if containsFold(ic.Payload.Content, m) {
			score.Politeness = 5
			break
		}
	}

	return score
}

// IntentAppreciIntent 辅助：把 Intent 转为 Sentiment 用于对比
//
// 实际不做转换；这里用 helper 处理 IntentAppreci（语义赞赏的意图）
func IntentAppreciIntent() Intent {
	return IntentGreeting
}

// containsFold 不区分大小写包含
func containsFold(s, substr string) bool {
	return len(s) >= len(substr) && (indexFold(s, substr) >= 0)
}

func indexFold(s, substr string) int {
	ls := toLower(s)
	lb := toLower(substr)
	for i := 0; i+len(lb) <= len(ls); i++ {
		if ls[i:i+len(lb)] == lb {
			return i
		}
	}
	return -1
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func formatFloat(f float64) string {
	intPart := int(f)
	frac := int((f - float64(intPart)) * 100)
	if frac < 0 {
		frac = -frac
	}
	if frac < 10 {
		return itoa(intPart) + ".0" + itoa(frac)
	}
	return itoa(intPart) + "." + itoa(frac)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

