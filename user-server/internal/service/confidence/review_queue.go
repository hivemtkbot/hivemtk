package confidence

// review_queue.go 边界审核队列服务
//
// 五层架构归属: L3 业务层
// 设计依据: docs/核心链路优化.md 第十五章 §15.4.12
//
// [0.6, 0.75) 区间的回复进队列，座席 30s 内可：
//   - 编辑后发出（标记 edited=true）
//   - 拦截转人工（标记 blocked=true）
//   - 不操作（30s 后自动发出，标记 auto_released=true）

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/repository"
)

// WebSocketBroadcaster WebSocket 推送接口
//
// 已有 WebSocketHub 满足，这里抽象为接口解耦
type WebSocketBroadcaster interface {
	// PushReviewItem 推送审核项给座席
	// agentID=0 表示推送给所有在线座席
	PushReviewItem(agentID uint, item *model.ReviewQueue) error
}

// ReviewQueueService 边界审核队列服务
type ReviewQueueService struct {
	repo          *repository.ReviewQueueRepository
	slaSeconds    int
	wsBroadcaster WebSocketBroadcaster
}

// NewReviewQueueService 创建审核队列服务
//
// slaSeconds 默认 30s
// wsBroadcaster 可为 nil（禁用 WebSocket 推送）
func NewReviewQueueService(repo *repository.ReviewQueueRepository, ws WebSocketBroadcaster) *ReviewQueueService {
	return &ReviewQueueService{
		repo:          repo,
		slaSeconds:    30,
		wsBroadcaster: ws,
	}
}

// SetSLASeconds 设置 SLA 秒数（热重载）
func (r *ReviewQueueService) SetSLASeconds(ctx context.Context, seconds int) {
	if seconds > 0 {
		r.slaSeconds = seconds
	}
}

// Enqueue 入队
//
// 流程：
//  1. 构造 ReviewQueue 记录，SLADeadline = now + slaSeconds
//  2. 写入 review_queue 表
//  3. 通过 WebSocket 推送给在线座席
func (r *ReviewQueueService) Enqueue(
	ctx context.Context,
	dec *dto.ConfidenceDecision,
	draftReply, sessionID, customerID, intentType string,
) (*model.ReviewQueue, error) {
	if dec == nil {
		return nil, fmt.Errorf("decision is nil")
	}
	now := time.Now()
	item := &model.ReviewQueue{
		ItemID:             uuid.New().String(),
		SessionID:          sessionID,
		CustomerID:         customerID,
		SignalID:           dec.SignalID,
		DraftReply:         draftReply,
		OriginalConfidence: dec.AggregatedConf,
		Threshold:          dec.DynamicThreshold,
		IntentType:         intentType,
		Status:             "pending",
		SLADeadline:        now.Add(time.Duration(r.slaSeconds) * time.Second),
	}
	if err := r.repo.Create(ctx, item); err != nil {
		return nil, err
	}
	// 推送给所有在线座席
	if r.wsBroadcaster != nil {
		_ = r.wsBroadcaster.PushReviewItem(0, item)
	}
	return item, nil
}

// Edit 座席编辑草稿
func (r *ReviewQueueService) Edit(ctx context.Context, itemID string, agentID uint, edited string) error {
	item, err := r.repo.GetByItemID(ctx, itemID)
	if err != nil || item == nil {
		return fmt.Errorf("review item not found: %s", itemID)
	}
	if item.Status != "pending" {
		return fmt.Errorf("item already acted: %s", item.Status)
	}
	now := time.Now()
	item.Status = "edited"
	item.AssignedAgentID = int64(agentID)
	item.EditedReply = edited
	item.AgentAction = "edit"
	item.ActedAt = &now
	return r.repo.Update(ctx, item)
}

// Block 座席拦截转人工
func (r *ReviewQueueService) Block(ctx context.Context, itemID string, agentID uint) error {
	item, err := r.repo.GetByItemID(ctx, itemID)
	if err != nil || item == nil {
		return fmt.Errorf("review item not found: %s", itemID)
	}
	if item.Status != "pending" {
		return fmt.Errorf("item already acted: %s", item.Status)
	}
	now := time.Now()
	item.Status = "blocked"
	item.AssignedAgentID = int64(agentID)
	item.AgentAction = "block"
	item.ActedAt = &now
	return r.repo.Update(ctx, item)
}

// ReleaseExpired 释放超时项（cron 每秒调用）
//
// 超时项标记为 auto_released，允许自动发出
// 返回释放的项数量
func (r *ReviewQueueService) ReleaseExpired(ctx context.Context) (int, error) {
	items, err := r.repo.ListPendingBefore(ctx, time.Now())
	if err != nil {
		return 0, err
	}
	released := 0
	for i := range items {
		items[i].Status = "auto_released"
		items[i].AutoReleased = true
		if err := r.repo.Update(ctx, &items[i]); err == nil {
			released++
		}
	}
	return released, nil
}

// PendingCount 当前队列长度（监控用）
func (r *ReviewQueueService) PendingCount(ctx context.Context) (int, error) {
	return r.repo.CountPending(ctx)
}

// GetByItemID 按 ItemID 查询
func (r *ReviewQueueService) GetByItemID(ctx context.Context, itemID string) (*model.ReviewQueue, error) {
	return r.repo.GetByItemID(ctx, itemID)
}

// ListByStatus 按 status 分页查询
func (r *ReviewQueueService) ListByStatus(ctx context.Context, status string, page, pageSize int) ([]model.ReviewQueue, int64, error) {
	return r.repo.ListByStatus(ctx, status, page, pageSize)
}
