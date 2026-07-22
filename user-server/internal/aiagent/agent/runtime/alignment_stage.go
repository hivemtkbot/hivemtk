package agent_runtime

import (
	"context"
	"time"
)

// ============================================================================
// Alignment Stage 6维拟人度打分阶段
// ----------------------------------------------------------------------------
// 文档依据：方向4 阶段3 同理心/热情/专业度等多轨打分
//
// 6 维评分：
//  1. Empathy   同理心 - 基于情绪（焦虑/愤怒 → 高同理心）
//  2. Enthusiasm 热情度 - 基于意图（询价/赞赏 → 高热情）
//  3. Expertise 专业度 - 知识库检索置信度（未挂知识库时取默认 3）
//  4. Patience  耐心   - 会话轮次（轮次越多越需要耐心）
//  5. Clarity   清晰度 - 表述简洁（无长难句 + 有问有答）
//  6. Politeness 礼貌度 - 敬语使用
// ============================================================================

// DefaultAlignmentScorer 默认 6 维拟人度评分器
type DefaultAlignmentScorer struct {
	// RAGConfidenceFunc 知识库检索置信度查询函数（可注入）
	// 返回 [0, 1]，未挂知识库返回 0
	RAGConfidenceFunc func(ctx context.Context, agentCtx *AgentContext, content string) float64
	// TurnCountFunc 当前会话已进行的轮次（可注入）
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

	// 1. Empathy: 情绪越负面 → 越要高同理心
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

	// 2. Enthusiasm: 询价/赞赏/销售线索 → 高热情
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

	// 3. Expertise: 知识库检索置信度
	if s.RAGConfidenceFunc != nil && ic.AgentCtx != nil {
		ragScore := s.RAGConfidenceFunc(ctx, ic.AgentCtx, ic.Payload.Content)
		// [0,1] → [1,5]
		score.Expertise = clamp(int(ragScore*4)+1, 1, 5)
	}

	// 4. Patience: 轮次越多越需耐心
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

	// 5. Clarity: 简单规则 — 消息长度适中（10-200字） → 高清晰度
	contentLen := len(ic.Payload.Content)
	switch {
	case contentLen >= 10 && contentLen <= 200:
		score.Clarity = 4
	case contentLen < 10:
		score.Clarity = 3 // 信息少
	default:
		score.Clarity = 3 // 长文本需要拆分
	}

	// 6. Politeness: 包含敬语 → 高礼貌
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
	// 我们没有 IntentAppreci，复用 IntentGreeting / IntentChitchat 表达赞赏语义
	// 此处保留接口以备后续扩展
	return IntentGreeting
}

// containsFold 不区分大小写包含
func containsFold(s, substr string) bool {
	return len(s) >= len(substr) && (indexFold(s, substr) >= 0)
}

func indexFold(s, substr string) int {
	// 简化：转小写后用 strings.Contains（性能可接受）
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
	// 简单格式化：保留 2 位小数
	intPart := int(f)
	frac := int((f - float64(intPart)) * 100)
	if frac < 0 {
		frac = -frac
	}
	// 用 string concat 避免 strconv 依赖（保持文件自包含）
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
