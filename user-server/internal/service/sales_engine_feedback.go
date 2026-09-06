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
