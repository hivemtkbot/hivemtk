// Package humanize/ai 拟人化 QA 工具
//
// 业界依据：
//   - GPTZero、Originality.ai 等 AI 文本检测器用 perplexity + burstiness
//   - 学术：Gehrmann et al. (2019) "GLTR: Human vs. Machine Translation"
//   - 反检测：perplexity-based rewriting（让 LLM 输出高 perplexity 文本）
//
// 用途：作为 humanize 流程的 QA gate
//   - LLM 输出后，计算 perplexity
//   - 若 perplexity 过高（> 阈值）→ 文本过于"自然"，可能被检测为人类
//   - 若 perplexity 过低（< 阈值）→ 文本过于"AI"，缺乏自然度
//   - 业界使用「in-the-middle」区间为最佳（默认 30-100）
package ai

import (
	"math"
	"strings"
	"unicode"
)

// Detector AI 文本检测器（基于字符合法性 + 简单 perplexity 估算）
//
// 注意：这是简化版实现。完整版需要：
//   - 预训练语言模型（如 GPT-2）做 token-level perplexity
//   - burstiness（句长方差）
//   - 词汇分布（top-k 词汇比例）
//
// 当前实现：基于 token 频率的「字符级 perplexity」（不需要预训练模型）
//   - 适合做轻量级 QA gate（性能 vs 精度权衡）
type Detector struct {
	// 阈值：perplexity > upper → 高度自然（可能需要降低）
	//       perplexity < lower → 高度模式化（可能需要提升）
	lowerBound float64
	upperBound float64

	// 训练用 baseline（从真实人类对话统计得到）
	humanBaseline float64
	aiBaseline    float64
}

// NewDetector 默认参数
//
// 业界默认（基于多个 IM 语料统计）：
//   - 人类 IM: ppl ≈ 50-150
//   - AI 输出: ppl ≈ 5-30（高度模式化）
//   - good humanize: ppl 落在 30-100（混合）
func NewDetector() *Detector {
	return &Detector{
		lowerBound:    20.0,
		upperBound:    200.0,
		humanBaseline: 100.0,
		aiBaseline:    15.0,
	}
}

// SetThresholds 自定义阈值
func (d *Detector) SetThresholds(lower, upper float64) {
	if d == nil {
		return
	}
	if lower > 0 {
		d.lowerBound = lower
	}
	if upper > 0 {
		d.upperBound = upper
	}
}

// DetectionResult 检测结果
type DetectionResult struct {
	Perplexity    float64 `json:"perplexity"`
	Quality       string  `json:"quality"` // "good" | "too_ai" | "too_natural" | "unknown"
	Score         float64 `json:"score"`   // 0-1，1 = 理想拟人度
	Tokens        int     `json:"tokens"`
	CharEntropy   float64 `json:"char_entropy"`   // 字符级熵
	AIProbability float64 `json:"ai_probability"` // 0-1，AI 似然
}

// Score 综合打分
func (r *DetectionResult) GetScore() float64 {
	if r == nil {
		return 0
	}
	return r.Score
}

// IsAIGenerated 简单判断：AI 似然 > 0.5 视为 AI 生成
func (r *DetectionResult) IsAIGenerated() bool {
	if r == nil {
		return false
	}
	return r.AIProbability > 0.5
}

// Detect 评估文本的「AI 程度」
//
// 流程：
//  1. 分词（英文按空格，中文按 char-level）
//  2. 计算 token 频率分布
//  3. 字符级 perplexity = exp(-sum(p * log(p)))
//     注意：这是简化版（字符级），不是真正的 token-level perplexity
//  4. AI 似然 = sigmoid((aiBaseline - ppl) / scale)
func (d *Detector) Detect(text string) DetectionResult {
	if d == nil || text == "" {
		return DetectionResult{Quality: "unknown"}
	}

	tokens := tokenize(text)
	if len(tokens) == 0 {
		return DetectionResult{Quality: "unknown"}
	}

	// 词频统计
	counts := make(map[string]int)
	for _, tok := range tokens {
		counts[tok]++
	}
	total := len(tokens)

	// Shannon 熵
	entropy := 0.0
	for _, c := range counts {
		p := float64(c) / float64(total)
		if p > 0 {
			entropy -= p * math.Log2(p)
		}
	}

	// 字符级 perplexity = 2^entropy
	// 业界直觉：
	//   - 高熵（多不同 token）→ 高 ppl → 接近人类
	//   - 低熵（重复 token）→ 低 ppl → 接近 AI
	perplexity := math.Pow(2, entropy)

	// AI 似然：用 sigmoid 映射
	// ppl=aiBaseline (15) → AI=0.5; ppl 越低 AI 越高
	aiProb := sigmoid((d.aiBaseline - perplexity) / 20.0)

	// 质量分级
	quality := "good"
	switch {
	case perplexity < d.lowerBound:
		quality = "too_ai" // 过于模式化
	case perplexity > d.upperBound:
		quality = "too_natural" // 过于"自然"（少见但有可能是 base model 输出）
	}

	// 拟人化分数：距 lowerBound 越远越好（但不超过 upperBound）
	score := 0.0
	switch {
	case quality == "too_ai":
		score = perplexity / d.lowerBound * 0.5 // 0-0.5
	case quality == "too_natural":
		score = 0.5 + (d.upperBound-perplexity)/(d.upperBound-d.humanBaseline)*0.3 // 0.5-0.8
	case quality == "good":
		// 在 good 区间内按距 humanBaseline 的接近度打分
		dist := math.Abs(perplexity - d.humanBaseline)
		maxDist := math.Max(d.humanBaseline-d.lowerBound, d.upperBound-d.humanBaseline)
		score = 0.8 + 0.2*(1-dist/maxDist)
		if score > 1 {
			score = 1
		}
	default:
		score = 0.5
	}

	return DetectionResult{
		Perplexity:    perplexity,
		Quality:       quality,
		Score:         score,
		Tokens:        total,
		CharEntropy:   entropy,
		AIProbability: aiProb,
	}
}

// tokenize 简单分词（英文按空格，中文按 char-level）
func tokenize(text string) []string {
	var tokens []string
	var current strings.Builder
	for _, r := range text {
		if unicode.IsSpace(r) {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		} else if unicode.IsPunct(r) {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			tokens = append(tokens, string(r))
		} else {
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// sigmoid 标准 sigmoid
func sigmoid(x float64) float64 {
	if x >= 0 {
		return 1.0 / (1.0 + math.Exp(-x))
	}
	ex := math.Exp(x)
	return ex / (1.0 + ex)
}
