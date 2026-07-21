package event

import (
	"encoding/json"

	"marketing/internal/model"
	"marketing/internal/repository"
)

// OperationLogSubscriber 操作日志订阅者
//
// 职责：
//   - 订阅 TopicOperationLog 主题
//   - 接收 OperationLogPayload 事件
//   - 异步写入 operation_logs 表
//
// 设计说明：
//   - 与 TeamUserService.logOperation 解耦，主流程不再等待日志写入完成
//   - 写入失败仅记录日志，不影响主业务（best-effort）
type OperationLogSubscriber struct {
	repo repository.OperationLogRepository
}

// NewOperationLogSubscriber 创建操作日志订阅者
func NewOperationLogSubscriber(repo repository.OperationLogRepository) *OperationLogSubscriber {
	return &OperationLogSubscriber{repo: repo}
}

// Handle 处理操作日志事件
func (s *OperationLogSubscriber) Handle(evt Event) error {
	payload, ok := evt.Payload.(OperationLogPayload)
	if !ok {
		return nil
	}

	oldValueJSON, _ := json.Marshal(payload.OldValue)
	newValueJSON, _ := json.Marshal(payload.NewValue)

	logEntry := &model.OperationLog{
		UserID:     payload.UserID,
		Username:   payload.Username,
		Action:     payload.Action,
		Module:     payload.Module,
		Resource:   payload.Resource,
		ResourceID: payload.ResourceID,
		OldValue:   string(oldValueJSON),
		NewValue:   string(newValueJSON),
		IP:         payload.IP,
	}

	return s.repo.Create(logEntry)
}
