package agent_runtime

import (
	"time"

	"marketing/internal/event"
)

// ============================================================================
// Publisher 事件发布器
// ----------------------------------------------------------------------------
// 设计依据: §2.2
// 用途:Channel Adapter(webhook 层)在收到消息后,调用本发布器发布事件
//       AgentRuntime.EventSubscriber 订阅并处理
//
// 优势:
//   1. Channel Adapter 不需要知道 AI 引擎存在
//   2. 新增渠道无需修改 AI 代码,只需 publish 事件
//   3. Event Bus 是天然的可观测点
// ============================================================================

// PublishCustomerMessage 发布客户消息事件
//
// 调用方:WebhookService.triggerSalesEngine / 各 Channel Adapter
//
// 参数:
//   - channelType: telegram / wecom / feishu / douyin / ...
//   - accountID:   渠道账号主键
//   - customerID:  客户 OneID(已归一化)
//   - sessionID:   会话唯一 ID(方向8 核心数据流必备,缺省由 channel:customer 构造)
//   - content:     消息内容
//   - traceID:     全链路追踪 ID(空字符串时自动生成)
//
// 返回:
//   - 生成的 TraceID(便于调用方记录到日志)
func PublishCustomerMessage(channelType, accountID, customerID, sessionID, content, traceID string) string {
	if traceID == "" {
		traceID = "agent_runtime_" + time.Now().Format("20060102150405.000000")
	}
	if sessionID == "" {
		sessionID = channelType + ":" + customerID
	}

	payload := event.CustomerMessagePayload{
		ChannelType: channelType,
		AccountID:   accountID,
		CustomerID:  customerID,
		SessionID:   sessionID,
		Content:     content,
		MessageType: "text",
		Timestamp:   time.Now(),
		TraceID:     traceID,
	}

	event.Publish(event.TopicCustomerMessageReceived, payload)
	return traceID
}

// PublishCustomerMessageWithType 发布客户消息事件(支持自定义消息类型)
//
// 用于图片/语音/事件 等非文本消息
func PublishCustomerMessageWithType(channelType, accountID, customerID, sessionID, content, messageType, traceID string) string {
	if traceID == "" {
		traceID = "agent_runtime_" + time.Now().Format("20060102150405.000000")
	}
	if sessionID == "" {
		sessionID = channelType + ":" + customerID
	}

	payload := event.CustomerMessagePayload{
		ChannelType: channelType,
		AccountID:   accountID,
		CustomerID:  customerID,
		SessionID:   sessionID,
		Content:     content,
		MessageType: messageType,
		Timestamp:   time.Now(),
		TraceID:     traceID,
	}

	event.Publish(event.TopicCustomerMessageReceived, payload)
	return traceID
}
