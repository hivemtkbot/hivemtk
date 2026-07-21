package service

import (
	"errors"
	"time"

	"marketing/internal/model"
	"marketing/internal/repository"
)

// RecoveryQueueService 流失挽回队列服务
type RecoveryQueueService struct {
	repo    repository.RecoveryQueueRepository
	nowFunc func() time.Time
}

// NewRecoveryQueueService 创建挽回队列服务
func NewRecoveryQueueService() *RecoveryQueueService {
	return &RecoveryQueueService{
		repo:    repository.NewRecoveryQueueRepository(),
		nowFunc: time.Now,
	}
}

// NewRecoveryQueueServiceWithRepo 测试用
func NewRecoveryQueueServiceWithRepo(r repository.RecoveryQueueRepository) *RecoveryQueueService {
	return &RecoveryQueueService{repo: r, nowFunc: time.Now}
}

// Enqueue 手动入队
func (s *RecoveryQueueService) Enqueue(customerID, unifiedID, account, reason, strategy string, priority int) (*model.RecoveryQueue, error) {
	if customerID == "" {
		return nil, errors.New("customer_id 不能为空")
	}
	if priority < 1 || priority > 10 {
		priority = 5
	}
	if reason == "" {
		reason = "churn"
	}
	if strategy == "" {
		strategy = "sms_coupon"
	}
	item := &model.RecoveryQueue{
		CustomerID:  customerID,
		UnifiedID:   unifiedID,
		Account:     account,
		Reason:      reason,
		Strategy:    strategy,
		Priority:    priority,
		Stage:       model.RecoveryStageQueued,
		MaxAttempts: 3,
	}
	if err := s.repo.Create(item); err != nil {
		return nil, err
	}
	return item, nil
}

// MarkAttempt 记录一次触达尝试
//
//	stage: succeed / failed / running
//	nextDelay: 下次重试延迟（0 表示不再重试）
func (s *RecoveryQueueService) MarkAttempt(id uint64, channel, result, stage string, nextDelay time.Duration) error {
	if id == 0 {
		return errors.New("id 不能为空")
	}
	if stage == "" {
		stage = model.RecoveryStageFailed
	}
	if err := s.repo.MarkAttempt(id, channel, result, nextDelayPtr(s.nowFunc(), nextDelay)); err != nil {
		return err
	}
	return s.repo.MarkStage(id, stage)
}

// MarkRecovered 标记为已挽回
func (s *RecoveryQueueService) MarkRecovered(id uint64, value int64) error {
	if id == 0 {
		return errors.New("id 不能为空")
	}
	item, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	now := s.nowFunc()
	item.RecoveredAt = &now
	item.RecoveryValue = value
	item.Stage = model.RecoveryStageSucceed
	return s.repo.Update(item)
}

// Cancel 取消入队
func (s *RecoveryQueueService) Cancel(id uint64) error {
	if id == 0 {
		return errors.New("id 不能为空")
	}
	return s.repo.MarkStage(id, model.RecoveryStageCancelled)
}

// ListByStage 按阶段分页
func (s *RecoveryQueueService) ListByStage(stage string, page, pageSize int) ([]*model.RecoveryQueue, int64, error) {
	return s.repo.ListByStage(stage, page, pageSize)
}

// Distribution 阶段统计
func (s *RecoveryQueueService) Distribution() (map[string]int64, error) {
	return s.repo.CountByStage()
}

// ListReadyForAttempt 取出可触达任务
func (s *RecoveryQueueService) ListReadyForAttempt(limit int) ([]*model.RecoveryQueue, error) {
	return s.repo.ListReadyForAttempt(s.nowFunc(), limit)
}

// nextDelayPtr 工具：nextDelay 为 0 时返回 nil，否则返回 now+nextDelay
func nextDelayPtr(now time.Time, nextDelay time.Duration) *time.Time {
	if nextDelay <= 0 {
		return nil
	}
	t := now.Add(nextDelay)
	return &t
}
