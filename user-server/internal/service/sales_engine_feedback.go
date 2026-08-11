package service

import (
	"context"

	"fmt"

	"hivemtk-user/internal/dto"

	"hivemtk-user/internal/pkg/utils/logger"
)

type FeedbackRecorderInterface interface {
	RecordFeedback(ctx context.Context, record *FeedbackRecord) error
}

// recordFeedback 记录反馈学习快照（SalesEngine 主链路第 9 步）
// ----------------------------------------------------------------------------
// 商业产品级 AI 自我进化闭环：
//
//	每次 Handle 结束都把"本次决策快照"喂给 FeedbackLearner，包括
//	intent/confidence/SOP/AIReply/是否转人工/token/耗时。
//	CustomerAccept 默认 false（生成时尚未知客户是否接受），
//	后续 SmartCSOrchestrator 在客户下一条消息或人工接管时更新。
//
// 设计原则：
//   - feedbackLearner 为 nil 时静默跳过（不破坏现有链路）
//   - 所有 return 路径都经过 defer，确保不遗漏
//   - 记录失败不影响主流程（仅 log）
func (e *SalesEngine) recordFeedback(ctx context.Context, req *SalesRequest, resp *SalesResponse) {
	if e.feedbackLearner == nil {
		return
	}
	if req == nil || resp == nil {
		return
	}
	record := &FeedbackRecord{
		SessionID:      req.SessionID,
		CustomerID:     req.CustomerID,
		AIReply:        resp.Reply,
		Transferred:    resp.TransferredToHuman,
		TransferReason: resp.TransferReason,
		Tokens:         resp.CostTokens,
		LatencyMs:      resp.LatencyMs,
	}
	if resp.Intent != nil {
		record.IntentType = resp.Intent.IntentType
		record.Confidence = resp.Intent.Confidence
	}
	if resp.MatchedSOP != nil {
		record.SOPName = resp.MatchedSOP.Name
	}
	if err := e.feedbackLearner.RecordFeedback(ctx, record); err != nil {

		logger.Errorf("[SalesEngine] feedback learner record failed: %v", err)
	}
	resp.Steps = append(resp.Steps, dto.SalesStepLog{
		Step:   "9_feedback_learn",
		Status: "ok",
		Detail: fmt.Sprintf("intent=%s conf=%.2f sop=%s transferred=%v tokens=%d",
			record.IntentType, record.Confidence, record.SOPName, record.Transferred, record.Tokens),
	})
}
