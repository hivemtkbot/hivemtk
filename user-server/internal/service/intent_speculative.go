package service

// intent_speculative.go 投机意图识别 (Phase 0 并行化)
//
// 设计依据: AI 智能体性能优化
//
// 问题: 7B Q5 本地 LLM 单次推理 1-3s, 串行执行会阻塞主流程
// 解法: 规则匹配 (O(1) < 1ms) 同步返回 + LLM 识别后台异步执行
//   - 同步返回: 规则结果立即可用,主流程可继续 Phase 1 (RAG/SOP/LLM 生成)
//   - 异步落库: LLM 完成后通过 channel 返回,主流程可选择性收割 (10ms 超时)
//
// 与 Recognize 的区别:
//   - Recognize: 规则未命中时同步等 LLM (3-15s 阻塞)
//   - RecognizeSpeculative: 规则未命中时立即返回 placeholder,LLM 后台跑

import (
	"context"
	"time"

	"marketing/internal/dto"
)

// RecognizeSpeculative 投机识别 (并行化优化)
//
// 返回:
//   - 同步结果 *dto.RecognizeResult: 规则命中即返回, 未命中返回低置信度 placeholder
//   - 异步 channel: LLM 完成后通过 channel 投递最终结果 (缓冲 1)
//   - error: 严重错误 (ctx canceled, dispatcher nil 等)
//
// 调用方建议:
//   1. 立即用同步结果继续 Phase 1 (SOP/RAG/LLM 生成候选)
//   2. Phase 2 异步收割 channel: select + 10ms timeout
//   3. 如果 LLM 结果置信度更高,可选择性升级 (用于下一轮 cache)
func (s *IntentRecognizer) RecognizeSpeculative(
	ctx context.Context, sessionID, customerID, text string,
) (*dto.RecognizeResult, <-chan *dto.RecognizeResult, error) {
	// 0. 空文本直接返回
	if text == "" {
		empty := &dto.RecognizeResult{IntentType: IntentUnknown, Confidence: 0, Method: "rule"}
		ch := make(chan *dto.RecognizeResult, 1)
		ch <- empty
		close(ch)
		return empty, ch, nil
	}

	// 1. 全局开关: 未开启意图识别
	if !IntentEnabled {
		placeholder := &dto.RecognizeResult{
			IntentType:      IntentUnknown,
			IntentName:      "未知",
			Confidence:      0.3,
			ConfidenceLevel: "low",
			Sentiment:       "neutral",
			Method:          "disabled",
		}
		ch := make(chan *dto.RecognizeResult, 1)
		ch <- placeholder
		close(ch)
		return placeholder, ch, nil
	}

	// 2. 规则匹配 (同步, O(1) < 1ms)
	if r := s.recognizeByRule(ctx, text); r != nil {
		// 规则命中: 立即返回规则结果, LLM 后台异步落库
		s.saveRecord(ctx, sessionID, customerID, text, r, "", 0, 0)
		ch := make(chan *dto.RecognizeResult, 1)
		if s.dispatcher != nil {
			go s.runLLMAsync(sessionID, customerID, text, ch, true /* ruleHit */)
		} else {
			// dispatcher 未配置, 关闭 channel
			ch <- r
			close(ch)
		}
		return r, ch, nil
	}

	// 3. 规则未命中: 返回低置信度 placeholder, LLM 异步跑
	placeholder := &dto.RecognizeResult{
		IntentType:      IntentUnknown,
		IntentName:      "未知",
		Confidence:      0.3,
		ConfidenceLevel: "low",
		Sentiment:       "neutral",
		Method:          "rule_placeholder",
	}
	ch := make(chan *dto.RecognizeResult, 1)
	if s.dispatcher != nil {
		go s.runLLMAsync(sessionID, customerID, text, ch, false /* ruleHit */)
	} else {
		ch <- placeholder
		close(ch)
	}
	return placeholder, ch, nil
}

// runLLMAsync LLM 异步执行 (后台 goroutine)
//   - 必须使用 background ctx, 避免父 ctx 取消导致记录丢失
//   - 完成后写 channel (非阻塞, buffer=1)
//   - ruleHit=true: 规则已命中, LLM 结果仅落库 (用于统计 + 后续学习)
//   - ruleHit=false: LLM 结果通过 channel 投递, 主流程可收割
func (s *IntentRecognizer) runLLMAsync(
	sessionID, customerID, text string,
	ch chan<- *dto.RecognizeResult,
	ruleHit bool,
) {
	defer close(ch)
	bgCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	llmR, err := s.recognizeByLLM(bgCtx, text)
	if err != nil || llmR == nil {
		// LLM 失败: 不投递 (channel 关闭即可)
		return
	}
	// 落库 (同步)
	s.saveRecord(bgCtx, sessionID, customerID, text, llmR, llmR.LLMModel, llmR.CostTokens, llmR.LatencyMs)
	// SOP 联动
	if customerID != "" {
		s.triggerSOPByIntent(bgCtx, customerID, sessionID, llmR.IntentType, llmR.Confidence)
	}
	// 投递 (非阻塞, 即使主流程不收割也不阻塞 LLM 协程)
	if ruleHit {
		// 规则已命中, 投递仅用于可能的 cache upgrade
		select {
		case ch <- llmR:
		default:
		}
	} else {
		ch <- llmR
	}
}
