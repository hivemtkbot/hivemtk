package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ============================================================================
// LLMJudge LLM-as-Judge 评估框架（v1.2 出海多语言方案 P2-1）
// ----------------------------------------------------------------------------
// 思路：用一个 LLM 充当"评分员"，对另一个 LLM 生成的客服回复做多维
// 质量打分。与 chrF++（字面匹配）互补：
//   - chrF++  擅长捕捉字面 n-gram 重合度，但对同义不同形的回复会低估
//   - LLMJudge擅长捕捉语义 / 术语 / 语气，但成本高、有非确定性
//
// 评估维度（默认）：
//   - fluency     ：目标语言下的流畅度
//   - accuracy    ：信息准确性
//   - terminology ：SKU / 价格 / 品牌名是否保留正确
//   - tone        ：语气是否专业友好
//
// 设计：
//   - LLMJudge 为接口，便于替换实现（默认 DefaultLLMJudge + mock 测试）
//   - DefaultLLMJudge 复用项目现有 LLM 服务（通过 LLMServiceInterface 抽象）
//   - enabled=false 时不影响主流程（EvalService 异步调用直接跳过）
// ============================================================================

// LLMJudge LLM 评分器接口。
type LLMJudge interface {
	Judge(ctx context.Context, req JudgeRequest) (*JudgeResult, error)
}

// JudgeRequest 评分请求。
type JudgeRequest struct {
	Query      string   // 用户问题
	Reference  string   // 参考答案（可选，为空时 LLM 仅依据自身判断）
	Candidate  string   // 待评估的生成回复
	TargetLang string   // 目标语言（如 "en" / "zh" / "ja"）
	Criteria   []string // 评估维度（默认 fluency/accuracy/terminology/tone）
}

// JudgeResult 评分结果。
type JudgeResult struct {
	OverallScore    float64            // 综合分 0.0~1.0
	DimensionScores map[string]float64 // 各维度分数 0.0~1.0
	Explanation     string             // 评分理由
	Issues          []string           // 发现的问题列表
}

// LLMServiceInterface LLM 服务接口。
//
// 与 ragcustomerservice.LLMServiceInterface 解耦：
//   - config 用 any 而非具体类型，避免 eval 包反向依赖 aiagent/llm
//   - 由调用方注入具体实现（如包装 llm.LLMService）
type LLMServiceInterface interface {
	Generate(ctx context.Context, config any, prompt string) (string, error)
}

// DefaultLLMJudge 默认 LLM 评分器实现。
//
// 通过注入 LLMServiceInterface 调用任意 OpenAI 兼容 LLM。
// enabled=false 时 Judge 直接返回 error，不发起 LLM 调用。
type DefaultLLMJudge struct {
	llmService LLMServiceInterface
	enabled    bool
}

// NewDefaultLLMJudge 创建默认 LLM 评分器（默认启用）。
//
// llmService 为 nil 时构造仍成功，但 Judge 调用会返回 error
// （便于在未配置 LLM 时仍能构造 EvalService）。
func NewDefaultLLMJudge(llmService LLMServiceInterface) *DefaultLLMJudge {
	return &DefaultLLMJudge{
		llmService: llmService,
		enabled:    true,
	}
}

// SetEnabled 启用/禁用评分器。
func (j *DefaultLLMJudge) SetEnabled(enabled bool) {
	j.enabled = enabled
}

// Enabled 返回当前是否启用。
func (j *DefaultLLMJudge) Enabled() bool {
	return j.enabled
}

// Judge 用 LLM 对候选回复进行多维评分。
//
// 失败场景：
//   - 评分器未启用：返回 errors.New("llm judge disabled")
//   - LLM 服务未注入：返回 errors.New("llm service not configured")
//   - LLM 调用失败：包裹原始 error
//   - 响应解析失败：包裹原始 error
func (j *DefaultLLMJudge) Judge(ctx context.Context, req JudgeRequest) (*JudgeResult, error) {
	if !j.enabled {
		return nil, errors.New("llm judge disabled")
	}
	if j.llmService == nil {
		return nil, errors.New("llm service not configured")
	}

	prompt := j.buildJudgePrompt(req)
	resp, err := j.llmService.Generate(ctx, nil, prompt)
	if err != nil {
		return nil, fmt.Errorf("llm judge: generate failed: %w", err)
	}
	return j.parseJudgeResult(resp)
}

// buildJudgePrompt 构造评分 prompt。
//
// 要求 LLM 仅返回 JSON 对象（无 markdown / 无额外解释），
// 便于后续 parseJudgeResult 解析。
func (j *DefaultLLMJudge) buildJudgePrompt(req JudgeRequest) string {
	reference := req.Reference
	if reference == "" {
		reference = "(no reference answer provided)"
	}
	return fmt.Sprintf(`You are a multilingual customer service quality evaluator.

Evaluate the following customer service reply based on these criteria:
- Fluency (0-1): How natural and fluent the reply is in %s
- Accuracy (0-1): How accurate the information is
- Terminology (0-1): Whether SKU/price/brand names are preserved correctly
- Tone (0-1): Whether the tone is professional and friendly

Customer question: %s
Reference answer: %s
Reply to evaluate: %s

Return ONLY a JSON object (no markdown, no explanation outside JSON):
{
  "overall_score": 0.0,
  "dimension_scores": {"fluency": 0.0, "accuracy": 0.0, "terminology": 0.0, "tone": 0.0},
  "explanation": "...",
  "issues": ["..."]
}`, req.TargetLang, req.Query, reference, req.Candidate)
}

// parseJudgeResult 解析 LLM 返回的 JSON 评分结果。
//
// 容错策略：
//   - LLM 可能返回带 markdown 代码块（```json ... ```）的内容，extractJSON 负责提取
//   - overall_score 若 > 1（某些 LLM 用 0-100 打分）自动除以 100 归一化
//   - 解析失败返回原始错误，不 panic
func (j *DefaultLLMJudge) parseJudgeResult(resp string) (*JudgeResult, error) {
	if strings.TrimSpace(resp) == "" {
		return nil, errors.New("llm judge: empty response")
	}
	jsonStr := extractJSON(resp)
	if jsonStr == "" {
		return nil, errors.New("llm judge: no JSON object found in response")
	}

	var raw struct {
		OverallScore    float64            `json:"overall_score"`
		DimensionScores map[string]float64 `json:"dimension_scores"`
		Explanation     string             `json:"explanation"`
		Issues          []string           `json:"issues"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("llm judge: parse json failed: %w", err)
	}

	// 归一化 overall_score 到 [0, 1]
	score := raw.OverallScore
	if score > 1.0 {
		score = score / 100.0
	}
	if score < 0 {
		score = 0
	}
	if score > 1.0 {
		score = 1.0
	}

	return &JudgeResult{
		OverallScore:    score,
		DimensionScores: raw.DimensionScores,
		Explanation:     raw.Explanation,
		Issues:          raw.Issues,
	}, nil
}

// extractJSON 从可能包含 markdown 代码块或额外文本的字符串中提取首个
// 完整的 JSON 对象（{ ... }）。
//
// 处理嵌套大括号与字符串内的转义字符。未找到返回空串。
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	depth := 0
	inStr := false
	escape := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if escape {
			escape = false
			continue
		}
		if c == '\\' {
			escape = true
			continue
		}
		if c == '"' {
			inStr = !inStr
			continue
		}
		if inStr {
			continue
		}
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}
