// Package behavioral 提供"行为层拟人"工具。
//
// 业界依据（Anthropic 2024 "Building effective agents" + 多项 UX 研究）：
//   - 文本层拟人（emoji / 语气词 / 句首安抚）效果有限（GPTZero 等检测器可识别）
//   - 行为层拟人（打字延迟 / 分条发送 / 偶尔的错别字）真实感更强
//   - WhatsApp/iMessage 等 IM 场景下人类本身就"分段发消息"，LLM 一次性输出大段文字会显假
//
// 设计原则：
//   - 函数式 / 无状态：所有行为都基于"输入文本 + 配置"产出"发送计划"
//   - 速率可调：每个场景可独立配置
//   - A/B 测试友好：每个行为开关可单独 toggle
package behavioral

import (
	"math/rand"
	"strings"
	"unicode"
)

// BehaviorConfig 行为拟人配置
//
// 行业默认（基于 WhatsApp Business IM 行为研究）：
//   - 打字速度：25 字符/秒（人类中位数）
//   - 思考停顿：3 秒（接续对话时）
//   - 分条阈值：80 字符（超过此长度倾向于分段）
//   - 分条最小间隔：1.5 秒
//   - 错别字概率：3%（1/33 字符；用于降低 AI 痕迹）
type BehaviorConfig struct {
	EnableTypingDelay   bool
	TypingSpeedCPS      float64
	ThinkingPauseSec    float64
	EnableMessageSplit  bool
	SplitThresholdChars int
	SplitMinIntervalSec float64
	EnableTypoInjection bool
	TypoProbability     float64

	// A11（ACM CHI'24 hesitation 研究）：匀速=机器人特征，变速+偶发犹豫更拟人
	TypingSpeedJitter float64
	HesitationProb    float64
	HesitationMinSec  float64
	HesitationMaxSec  float64
}

// DefaultBehaviorConfig 返回默认行为配置
func DefaultBehaviorConfig() BehaviorConfig {
	return BehaviorConfig{
		EnableTypingDelay:   true,
		TypingSpeedCPS:      25.0,
		ThinkingPauseSec:    3.0,
		EnableMessageSplit:  true,
		SplitThresholdChars: 80,
		SplitMinIntervalSec: 1.5,
		EnableTypoInjection: false,
		TypoProbability:     0.03,

		TypingSpeedJitter: 0.2,
		HesitationProb:    0.15,
		HesitationMinSec:  0.4,
		HesitationMaxSec:  1.2,
	}
}

// SendPlan 一次"发送计划"：包含若干条消息和它们之间的时间间隔
type SendPlan struct {
	Messages   []string
	Intervals  []float64
	TotalDelay float64
}

// PlanSend 根据行为配置产出"发送计划"。
//
// 业界用法：
//   - 文本短（< SplitThresholdChars）：原样返回，附加 TypingDelay
//   - 文本长：按句子边界分段，片段间按 SplitMinIntervalSec 间隔
//   - 接续对话（非首条）：附加 ThinkingPauseSec 思考停顿
//
// rng: 可注入的随机源（用于分条位置选择 / 错别字注入），nil 时用全局 rand
func PlanSend(text string, cfg BehaviorConfig, isFirstMessage bool, rng *rand.Rand) SendPlan {
	if rng == nil {
		rng = rand.New(rand.NewSource(0xBEEF))
	}
	if text == "" {
		return SendPlan{}
	}

	plan := SendPlan{}

	if cfg.EnableMessageSplit && shouldSplit(text, cfg.SplitThresholdChars) {
		plan.Messages = splitByPunctuation(text, rng)
	} else {
		plan.Messages = []string{text}
	}

	if len(plan.Messages) > 1 {
		plan.Intervals = make([]float64, len(plan.Messages)-1)
		for i := range plan.Intervals {

			jitter := (1 - cfg.TypingSpeedJitter) + 2*cfg.TypingSpeedJitter*rng.Float64()
			interval := cfg.SplitMinIntervalSec * jitter

			if cfg.HesitationProb > 0 && rng.Float64() < cfg.HesitationProb {
				lo, hi := cfg.HesitationMinSec, cfg.HesitationMaxSec
				if hi < lo {
					lo, hi = hi, lo
				}
				interval += lo + (hi-lo)*rng.Float64()
			}
			plan.Intervals[i] = interval
		}
	}

	if cfg.EnableTypoInjection && cfg.TypoProbability > 0 {
		for i := range plan.Messages {
			plan.Messages[i] = injectTypos(plan.Messages[i], cfg.TypoProbability, rng)
		}
	}

	if cfg.EnableTypingDelay {

		var total float64
		if !isFirstMessage {
			total += cfg.ThinkingPauseSec
		}
		for i, msg := range plan.Messages {
			j := cfg.TypingSpeedJitter
			speedCPS := cfg.TypingSpeedCPS
			if j > 0 {

				speedCPS = speedCPS * ((1 - j) + 2*j*rng.Float64())
			}
			total += typingTime(msg, speedCPS)
			if i < len(plan.Intervals) {
				total += plan.Intervals[i]
			}
		}
		plan.TotalDelay = total
	} else {

		for _, iv := range plan.Intervals {
			plan.TotalDelay += iv
		}
	}

	return plan
}

func shouldSplit(text string, threshold int) bool {

	return len([]rune(text)) > threshold
}

func splitByPunctuation(text string, rng *rand.Rand) []string {

	strongSplitters := []rune{'。', '!', '?', '！', '?', '.', '\n'}

	var segments []string
	var current []rune
	for _, r := range text {
		current = append(current, r)
		if containsRune(strongSplitters, r) {
			seg := strings.TrimSpace(string(current))
			if seg != "" {
				segments = append(segments, seg)
			}
			current = nil
		}
	}
	if rem := strings.TrimSpace(string(current)); rem != "" {
		segments = append(segments, rem)
	}

	if len(segments) < 2 {
		weakSplitters := []rune{'，', ',', ';', '；'}
		segments = nil
		current = nil
		for _, r := range text {
			current = append(current, r)
			if containsRune(weakSplitters, r) {
				seg := strings.TrimSpace(string(current))
				if seg != "" {
					segments = append(segments, seg)
				}
				current = nil
			}
		}
		if rem := strings.TrimSpace(string(current)); rem != "" {
			segments = append(segments, rem)
		}
	}

	merged := make([]string, 0, len(segments))
	var buf string
	for _, s := range segments {
		if len([]rune(s)) < 8 && buf != "" {
			buf += s
		} else if len([]rune(s)) < 8 {
			buf = s
		} else {
			if buf != "" {
				merged = append(merged, buf)
				buf = ""
			}
			merged = append(merged, s)
		}
	}
	if buf != "" {
		merged = append(merged, buf)
	}

	if len(merged) < 2 {
		return []string{text}
	}
	return merged
}

func typingTime(text string, speedCPS float64) float64 {
	if speedCPS <= 0 {
		speedCPS = 25.0
	}
	return float64(len([]rune(text))) / speedCPS
}

func injectTypos(text string, prob float64, rng *rand.Rand) string {
	if prob <= 0 {
		return text
	}

	adjacent := map[rune]string{
		'a': "s", 'b': "vn", 'c': "xv", 'd': "sf", 'e': "wr", 'f': "dg",
		'g': "fh", 'h': "gj", 'i': "uo", 'j': "hk", 'k': "jl", 'l': "k",
		'm': "n", 'n': "bm", 'o': "ip", 'p': "o", 'q': "w", 'r': "et",
		's': "ad", 't': "ry", 'u': "yi", 'v': "cb", 'w': "qe", 'x': "zc",
		'y': "tu", 'z': "x",
	}
	out := make([]rune, 0, len(text))
	for _, r := range text {
		if unicode.IsLetter(r) && unicode.IsLower(r) && rng.Float64() < prob {
			if alts, ok := adjacent[r]; ok && len(alts) > 0 {
				r = rune(alts[rng.Intn(len(alts))])
			}
		}
		out = append(out, r)
	}
	return string(out)
}

func containsRune(haystack []rune, needle rune) bool {
	for _, r := range haystack {
		if r == needle {
			return true
		}
	}
	return false
}
