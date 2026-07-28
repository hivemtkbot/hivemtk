// 统一协议常量与消息工厂（与服务端 user-server/internal/bridge/frames.go 严格一致）
//
// ⚠️ 协议即契约：服务端是稳定方（还承担 ingress/AI 流水线），扩展作为客户端必须对齐服务端。
// 历史教训：首版曾因常量/字段名不一致（inbound vs inbound_message、text vs content）
// 导致消息流端到端不通。本文件是唯一真相源，任何改动需同步校验 server 端 frames.go。

import { PROTOCOL, RATE_LIMIT_DEFAULTS } from './constants.js';

// 协议字段：导出 constants.js 中的 PROTOCOL（保持单源）
export const CHANNELS = PROTOCOL.CHANNELS;

// 帧类型常量（与服务端 Frame* 常量一一对应）
export const FRAME = PROTOCOL.FRAME;

// 消息发送方类型
export const SENDER = PROTOCOL.SENDER;

// 消息方向（history 帧专用；inbound_message 由服务端固定为 inbound）
export const DIRECTION = PROTOCOL.DIRECTION;

// 默认风控/限速参数：从 constants.js 单源 re-export，禁止就地修改
// 文档源：docs/bridge/DEFAULTS.md，详见 bridge.md §17.3
export { RATE_LIMIT_DEFAULTS };

// 构造上行消息（统一模型，字段名与服务端 UnifiedMessage 完全一致）
// sender_id 规则：客户消息 = conversation_id（对方）；自己/AI 消息 = account_id。
export function makeUnifiedMessage({
  channel,
  account_id,
  conversation_id,
  event_id,
  sender_type = SENDER.CUSTOMER,
  sender_id,
  sender_name = '',
  receiver_id = '',
  msg_type = 'text',
  content = '',
  media_url = '',
  timestamp,
  direction,
  raw = null,
}) {
  const resolvedSenderId = sender_id || (sender_type === SENDER.CUSTOMER ? conversation_id : account_id);
  return {
    channel,
    account_id,
    conversation_id,
    event_id: event_id || `${channel}:${conversation_id}:${Date.now()}:${Math.random().toString(36).slice(2, 8)}`,
    sender_type,
    sender_id: resolvedSenderId,
    sender_name,
    receiver_id,
    msg_type,
    content: content || '',
    media_url: media_url || '',
    timestamp: timestamp || Date.now(),
    direction, // 仅 history 帧使用；inbound_message 服务端忽略
    raw,
  };
}

// 解析下行回复（统一模型，字段名与服务端 UnifiedReply 完全一致）
export function parseUnifiedReply(reply) {
  return {
    channel: reply.channel,
    account_id: reply.account_id,
    conversation_id: reply.conversation_id,
    msg_type: reply.msg_type || 'text',
    content: reply.content || '',
    media_url: reply.media_url || '',
    reply_to_event_id: reply.reply_to_event_id || '',
  };
}
