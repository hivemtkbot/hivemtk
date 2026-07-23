package agent_runtime

import (
	"context"
	"errors"
	"runtime/debug"
	"time"

	"marketing/internal/event"
	"marketing/internal/pkg/utils/logger"
)

// ============================================================================
// EventSubscriber 事件订阅者实现
// ----------------------------------------------------------------------------
// 订阅 customer.message.received 主题
// 调 AgentRuntime.HandleCustomerMessage → SalesEngine / SmartCSOrchestrator
//
// 设计依据:ADR-008 §2.2
// 关联文档:MULTI_AI_AGENT_DESIGN §9
// ============================================================================

// eventBusEventSubscriber 事件订阅者
type eventBusEventSubscriber struct {
	runtime AgentRuntime
}

// NewEventSubscriber 创建事件订阅者
//
// 调用方:cmd/api/main.go 启动阶段
// 注册方式:
//
//	bus := event.GetGlobalBus()
//	bus.Subscribe(event.TopicCustomerMessageReceived, agent_runtime.NewEventSubscriber(rt))
func NewEventSubscriber(rt AgentRuntime) event.Handler {
	return (&eventBusEventSubscriber{runtime: rt}).Handle
}

// Handle 处理事件总线消息
//
// 步骤:
//  1. 类型断言:event.Payload 必须是 CustomerMessagePayload
//  2. 调用 runtime.HandleCustomerMessage
//  3. 错误处理:仅记录日志,不重试(主流程已 by design 不阻塞)
//
// 异常隔离:用 defer + recover 防止单个事件 panic 影响整个 worker
func (s *eventBusEventSubscriber) Handle(evt event.Event) error {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[agent_runtime] panic in event handler topic=%s err=%v\n%s",
				evt.Topic, r, debug.Stack())
		}
	}()

	// 类型断言
	payload, ok := evt.Payload.(event.CustomerMessagePayload)
	if !ok {
		// 兼容:某些调用方可能传指针
		if p, ok := evt.Payload.(*event.CustomerMessagePayload); ok {
			if p == nil {
				return errors.New("event payload is nil")
			}
			// 事件总线订阅不持有上游 ctx,使用 background ctx
			return s.handleMessage(context.Background(), *p)
		}
		return errors.New("invalid event payload type for " + evt.Topic)
	}

	// 事件总线订阅不持有上游 ctx,使用 background ctx
	return s.handleMessage(context.Background(), payload)
}

// handleMessage 处理单条客户消息
func (s *eventBusEventSubscriber) handleMessage(ctx context.Context, payload event.CustomerMessagePayload) error {
	// R3/V3 幂等守卫（只读侧）：若同步主链路(webhook.sendOutbound)已认领该
	// EventID(TraceID) 出站，总线直接跳过，退化为纯观察，绝不抢发送权导致客户漏回或重复回复。
	// 注：webhook 调用 PublishCustomerMessage 时把 ParsedPayload.EventID 填入 TraceID，
	// 故此处以 payload.TraceID 作为与同步主链路一致的幂等键。
	// 当前总线订阅默认关闭(AGENT_RUNTIME_BUS_ENABLED!=true)，此守卫为防御纵深：
	// 未来开启双写且总线真正出站时，应在出站成功后调用 MarkReplied。
	if HasReplied(payload.TraceID) {
		logger.Infof("[agent_runtime] event=%s already replied by sync path, skip bus handling", payload.TraceID)
		return nil
	}

	start := time.Now()

	// 缺省 TraceID
	if payload.TraceID == "" {
		payload.TraceID = "agent_runtime_" + start.Format("20060102150405.000000")
	}

	// 类型转换:event.CustomerMessagePayload → agent_runtime.CustomerMessagePayload
	rtPayload := convertFromEventPayload(payload)

	// 调 AgentRuntime 统一入口
	resp, err := s.runtime.HandleCustomerMessage(ctx, rtPayload)
	if err != nil {
		logger.Errorf("[agent_runtime] handle failed channel=%s account=%s customer=%s trace=%s err=%v",
			payload.ChannelType, payload.AccountID, payload.CustomerID, payload.TraceID, err)
		return err
	}

	// 记录响应(用于可观测性)
	duration := time.Since(start)
	if resp != nil {
		logger.Infof("[agent_runtime] handled channel=%s account=%s customer=%s trace=%s agent=%s confidence=%.2f tools=%v handoff=%v duration=%s",
			payload.ChannelType, payload.AccountID, payload.CustomerID, payload.TraceID,
			resp.AgentCode, resp.Confidence, resp.ToolsCalled, resp.HandoffToHuman, duration)
	}

	return nil
}

// convertFromEventPayload event 包载荷 → agent_runtime 内部载荷
//
// 避免 agent_runtime 强依赖 event 包的具体类型
// 当前两者结构相同,直接转换
func convertFromEventPayload(ep event.CustomerMessagePayload) CustomerMessagePayload {
	return CustomerMessagePayload{
		ChannelType:	ep.ChannelType,
		AccountID:	ep.AccountID,
		CustomerID:	ep.CustomerID,
		SessionID:	ep.SessionID,
		Content:	ep.Content,
		MessageType:	ep.MessageType,
		Timestamp:	ep.Timestamp,
		TraceID:	ep.TraceID,
		Raw:		ep.Raw,
	}
}
