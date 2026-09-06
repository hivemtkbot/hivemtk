package service

// reach_pipeline_job.go 触达任务的入队、生命周期管理（查询/取消/重试）与统计查询。

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
)

type EnqueueJobRequest struct {
	PipelineID uint           `json:"pipeline_id" binding:"required"`
	Channel    string         `json:"channel"`
	CustomerID string         `json:"customer_id" binding:"required"`
	AccountID  string         `json:"account_id"`
	Payload    map[string]any `json:"payload" binding:"required"`
	MaxRetry   int            `json:"max_retry"`
	RunAt      *time.Time     `json:"run_at"`
}

func (s *ReachPipelineService) EnqueueJob(ctx context.Context, req *EnqueueJobRequest) (*model.ReachJob, error) {
	if s.repo == nil || !s.repo.Available() {
		return nil, fmt.Errorf("db is nil")
	}
	if req.Payload == nil {
		return nil, ErrReachInvalidPayload
	}
	pipe, err := s.GetPipeline(ctx, req.PipelineID)
	if err != nil {
		return nil, err
	}
	if pipe.Status != PipelineStatusActive {
		return nil, fmt.Errorf("pipeline is not active")
	}
	channel := req.Channel
	if channel == "" {
		channel = pipe.Channel
	}
	if !ReachChannels[channel] {
		return nil, ErrReachInvalidChannel
	}

	maxRetry := req.MaxRetry
	if maxRetry <= 0 {
		maxRetry = 3
		if pipe.RetryPolicy != nil {
			var rp RetryPolicy
			if err := json.Unmarshal(mustJSON(pipe.RetryPolicy), &rp); err != nil {
				logger.Errorf("[reach_pipeline] 解析 RetryPolicy 失败: %v", err)
			}
			if rp.MaxRetries > 0 {
				maxRetry = rp.MaxRetries
			}
		}
	}

	payloadMap := model.JSONMap{}
	payloadBytes, _ := json.Marshal(req.Payload)
	if err := json.Unmarshal(payloadBytes, &payloadMap); err != nil {
		logger.Errorf("[reach_pipeline] 解析 Payload 失败: %v", err)
	}
	job := &model.ReachJob{

		PipelineID: req.PipelineID,
		Channel:    channel,
		CustomerID: req.CustomerID,
		AccountID:  req.AccountID,
		Payload:    payloadMap,
		State:      JobStatePending,
		MaxRetry:   maxRetry,
		NextRunAt:  req.RunAt,
	}
	if job.Payload == nil {
		job.Payload = model.JSONMap{}
	}
	if job.NextRunAt == nil {
		now := time.Now()
		job.NextRunAt = &now
	}
	if err := s.repo.CreateJob(ctx, job); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *ReachPipelineService) GetJob(ctx context.Context, id uint) (*model.ReachJob, error) {
	job, err := s.repo.FindJobByID(ctx, id)
	if err != nil {
		return nil, ErrReachJobNotFound
	}
	return job, nil
}

func (s *ReachPipelineService) ListJobs(ctx context.Context, channel, state string, page, pageSize int) ([]model.ReachJob, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	return s.repo.ListJobs(ctx, channel, state, page, pageSize)
}

func (s *ReachPipelineService) CancelJob(ctx context.Context, id uint) error {
	rowsAffected, err := s.repo.UpdateJobStateWithCond(ctx, id,
		[]string{JobStatePending, JobStateRunning, JobStateRetrying, JobStateRateLimited},
		map[string]any{
			"state":        JobStateCanceled,
			"completed_at": time.Now(),
		})
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrReachJobNotPending
	}
	return nil
}

func (s *ReachPipelineService) RetryJob(ctx context.Context, id uint) error {
	rowsAffected, err := s.repo.UpdateJobStateWithCond(ctx, id,
		[]string{JobStateFailed},
		map[string]any{
			"state":         JobStatePending,
			"next_run_at":   time.Now(),
			"error_message": "",
		})
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrReachJobNotPending
	}
	return nil
}

// ListJobsByExperiment 按实验 ID 关联查询触达任务
func (s *ReachPipelineService) ListJobsByExperiment(ctx context.Context, experimentID string, page, pageSize int) ([]model.ReachJob, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	return s.repo.ListJobs(ctx, experimentID, "", page, pageSize)
}

// Stats 统计
func (s *ReachPipelineService) Stats(ctx context.Context) (map[string]int64, error) {
	stats := map[string]int64{
		"total":        0,
		"active":       0,
		"paused":       0,
		"jobs":         0,
		"pending":      0,
		"running":      0,
		"success":      0,
		"failed":       0,
		"rate_limited": 0,
		"canceled":     0,
	}
	if s.repo == nil || !s.repo.Available() {
		return stats, nil
	}

	rs, err := s.repo.GetStats(ctx)
	if err != nil {
		return nil, err
	}
	stats["total"] = rs.TotalPipelines
	stats["active"] = rs.ActivePipelines
	stats["paused"] = rs.PausedPipelines
	stats["jobs"] = rs.TotalJobs
	stats["pending"] = rs.PendingJobs
	stats["running"] = rs.RunningJobs
	stats["success"] = rs.SuccessJobs
	stats["failed"] = rs.FailedJobs
	stats["rate_limited"] = rs.RateLimitedJobs
	stats["canceled"] = rs.CanceledJobs
	return stats, nil
}
