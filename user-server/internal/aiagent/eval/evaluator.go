package eval

import (
	"fmt"
	"time"
)

// ============================================================================
// Evaluator 多语言评估流水线（v1.2 出海多语言方案 P2-1）
// ----------------------------------------------------------------------------
// 组合 ChrFEvaluator（字面匹配）与 LLMJudge（语义评分），提供：
//   - 批量评估：跑 chrF++ 得到均分 + 每条样本分
//   - 单条评估：chrF++ 单点分数（LLMJudge 由上层 EvalService 异步触发）
//
// 设计：
//   - chrF 为必选组件（nil 时自动用默认参数构造）
//   - judge 为可选组件（nil 时仅做 chrF 评估）
//   - 评估是同步阻塞调用；异步抽样由 service/i18n.EvalService 负责
// ============================================================================

// EvaluationResult 批量评估结果。
type EvaluationResult struct {
	ChrF        float64       // 全批次平均 chrF++ 分数
	SampleCount int           // 样本数
	PerSample   []SampleScore // 每条样本的分数（顺序与输入一致）
	LangPair    string        // 语言对，如 "zh-en"
	Timestamp   time.Time     // 评估时间
}

// SampleScore 单条样本评分。
type SampleScore struct {
	Query     string  // 用户问题
	Reference string  // 标准答案
	Candidate string  // LLM 生成
	ChrF      float64 // chrF++ 分数
}

// Evaluator 多语言评估器。
type Evaluator struct {
	chrF  *ChrFEvaluator
	judge LLMJudge // 可选
}

// NewEvaluator 创建评估器。
//
// chrF 为 nil 时使用默认参数（CharN=6, WordN=2, Beta=2.0）。
// judge 为 nil 时仅支持 chrF 评估（EvaluateSingle / EvaluateBatch 不调用 LLM）。
func NewEvaluator(chrF *ChrFEvaluator, judge LLMJudge) *Evaluator {
	if chrF == nil {
		chrF = NewChrFEvaluator()
	}
	return &Evaluator{chrF: chrF, judge: judge}
}

// ChrF 返回底层 chrF++ 评估器（供调用方调参）。
func (e *Evaluator) ChrF() *ChrFEvaluator {
	return e.chrF
}

// Judge 返回底层 LLMJudge（可能为 nil）。
func (e *Evaluator) Judge() LLMJudge {
	return e.judge
}

// EvaluateBatch 批量评估：对每条样本计算 chrF++，返回均分 + 逐条分数。
//
// candidates 与 references 长度必须一致，否则返回 error。
// LangPair 仅作为结果元数据，不影响计算。
func (e *Evaluator) EvaluateBatch(candidates, references []string, langPair string) (*EvaluationResult, error) {
	if len(candidates) != len(references) {
		return nil, fmt.Errorf("eval: candidates(%d) and references(%d) length mismatch", len(candidates), len(references))
	}

	result := &EvaluationResult{
		LangPair:  langPair,
		Timestamp: time.Now(),
	}
	if len(candidates) == 0 {
		return result, nil
	}

	result.PerSample = make([]SampleScore, 0, len(candidates))
	var sum float64
	for i := 0; i < len(candidates); i++ {
		score := e.chrF.Score(candidates[i], references[i])
		sum += score
		result.PerSample = append(result.PerSample, SampleScore{
			Reference: references[i],
			Candidate: candidates[i],
			ChrF:      score,
		})
	}
	result.ChrF = sum / float64(len(candidates))
	result.SampleCount = len(candidates)
	return result, nil
}

// EvaluateSingle 单条评估：返回 chrF++ 分数。
//
// query 仅作为元数据（当前 chrF 计算不依赖 query），保留参数供未来
// 接入 LLMJudge 同步评分时使用。
func (e *Evaluator) EvaluateSingle(candidate, reference, query string) (float64, error) {
	return e.chrF.Score(candidate, reference), nil
}
