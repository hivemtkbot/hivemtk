// 统一协议常量与消息工厂（与服务端 user-server/internal/bridge/frames.go 严格一致）
//
// ⚠️ 协议即契约：服务端是稳定方（还承担 ingress/AI 流水线），扩展作为客户端必须对齐服务端。
// 历史教训：首版曾因常量/字段名不一致（inbound vs inbound_message、text vs content）
// 导致消息流端到端不通。本文件是唯一真相源，任何改动需同步校验 server 端 frames.go。

export const CHANNELS = {
  DOUYIN: 'douyin_web',
  XHS: 'xhs_web',
  TIKTOK: 'tiktok_web',
};

// 帧类型常量（与服务端 Frame* 常量一一对应）
export const FRAME = {
  REGISTER: 'register',          // 扩展上行：注册 channel+account
  INBOUND: 'inbound_message',    // 扩展上行：实时新私信（触发 AI）
  HISTORY: 'history',            // 扩展上行：历史/回填消息（仅落库，不触发 AI）
  OUTBOUND: 'outbound_reply',    // 服务器下行：AI 回复（回写网页）
  PONG: 'pong',                  // 扩展上行：保活
  PING: 'ping',                  // 服务器下行：保活
  ACK: 'ack',                    // 扩展上行：下行确认
  ERROR: 'error',                // 服务器下行：错误
};

// 消息发送方类型
export const SENDER = {
  CUSTOMER: 'customer', // 平台私信对方（客户）
  AGENT: 'agent',       // 本扩展/AI 回写的消息
  SELF: 'self',         // 账号主自己发的
};

// 消息方向（history 帧专用；inbound_message 由服务端固定为 inbound）
export const DIRECTION = {
  INBOUND: 'inbound',
  OUTBOUND: 'outbound',
};

// 默认风控/限速参数（可在 popup 配置覆盖；详见 bridge.md §17.3）
export const RATE_LIMIT_DEFAULTS = {
  // 单账号每分钟最大下行回复数（Token 桶容量）
  accountCapacity: 12,
  // 单账号每分钟补充速率
  accountRefillPerMin: 12,
  // 任意两次下行之间最小间隔（毫秒），拟人节奏
  minIntervalMs: 1500,
  // 同一会话两次回复之间冷却（毫秒），防回环/刷屏
  conversationCooldownMs: 3000,
  // 同一会话每小时最大回复数
  conversationPerHour: 40,
  // 拟人延迟抖动区间（毫秒），发送前随机等待
  jitterMinMs: 800,
  jitterMaxMs: 2600,
  // 同一会话相同文案去重窗口（毫秒）
  dedupWindowMs: 60000,
};

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
