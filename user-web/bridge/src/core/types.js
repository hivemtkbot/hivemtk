// 统一协议常量与消息工厂（与服务端 user-server/internal/bridge/frames.go 严格一致）
//
// ⚠️ 协议即契约：服务端是稳定方（还承担 ingress/AI 流水线），扩展作为客户端必须对齐服务端。
// 历史教训：首版曾因常量/字段名不一致（inbound vs inbound_message、text vs content）
// 导致消息流端到端不通。本文件是唯一真相源，任何改动需同步校验 server 端 frames.go。

import { PROTOCOL, RATE_LIMIT_DEFAULTS, PATROL_DEFAULTS } from './constants.js';

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

// 巡检制度默认参数：从 constants.js 单源 re-export
export { PATROL_DEFAULTS };

// 实时 inbound 帧携带的「会话上下文窗口」长度：让「一条消息 = 一个会话、内含多轮历史」成立，
// AI 侧得到该会话最近 N 轮作为上下文。服务端也可自取已存历史，窗口仅作即时上下文与冗余。
export const HISTORY_CONTEXT_WINDOW = 20;

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
  is_group = false,
  group_id = '',
  group_name = '',
  raw = null,
  history,
}) {
  const resolvedSenderId = sender_id || (sender_type === SENDER.CUSTOMER ? conversation_id : account_id);
  // 群聊 / 非文字消息等扩展字段：顶层输出（服务端 UnifiedMessage 已加 is_group/group_id/group_name）
  // 同时保留 Extra 冗余，确保老版本后端仍能读到。
  const extra = {};
  if (is_group) extra.is_group = true;
  if (group_id) extra.group_id = group_id;
  if (group_name) extra.group_name = group_name;
  if (sender_name && sender_type === SENDER.CUSTOMER) extra.sender_name = sender_name;
  // 「一个会话一个消息」：history 帧/实时 inbound 可携带该会话的多轮历史（每条含 direction）。
  // 仅当非空数组时输出，保持单条消息帧的向后兼容。
  const historyList = Array.isArray(history) && history.length > 0 ? history : undefined;
  // event_id 由调用方显式提供（DOM data-message-id 或 `c:${text}` 兜底），后端按 event_id 幂等去重。
  // 自/他判定已移交后端（服务端内容回显检测 + sender_type 强制覆盖），前端不再计算内容 hash。
  const stableEventId = event_id;
  return {
    channel,
    account_id,
    conversation_id,
    event_id: stableEventId,
    sender_type,
    sender_id: resolvedSenderId,
    sender_name,
    receiver_id,
    msg_type,
    content: content || '',
    media_url: media_url || '',
    timestamp: timestamp || Date.now(),
    direction, // 仅 history 帧使用；inbound_message 服务端忽略
    is_group: !!is_group,
    group_id: group_id || '',
    group_name: group_name || '',
    raw,
    history: historyList,
    extra: Object.keys(extra).length ? extra : undefined,
  };
}

// 消息内容哈希（与服务端 internal/service/webhook.go::ContentHashMsgID 严格一致）。
//
// ⚠️ 回环去重的契约核心（2026-08-07 审计修复 + 跨会话去重）：
//   服务端 AI 出站消息的 MsgID = ContentHashMsgID(channel, content) ← 不含 conversationID
//   = `mh:` + FNV-1a 32位(channel|trim(content))。
//   前端 patrol 扫描到 AI 回复消息并重新上报时必须以相同算法生成 event_id，
//   后端 GetByMsgID 才能命中 → 幂等跳过。同一文本无论在哪个会话被捕获，哈希一致。
//   算法必须前后端逐字节一致（输入顺序、trim、前缀、hex 格式），否则回环防护断裂。
//
//   合约锚点（2026-08-07 更新：去掉 conversationID）：
//     contentHash("douyin", "c1", "你好") === "mh:00550fed"
//
// ⚠️ 关键：FNV-1a 是按字节哈希。Go 端 []byte(s) 是 UTF-8 字节；JS 的 String.charCodeAt 是 UTF-16
//   码元，对多字节字符（中文等）结果不同 → 必须先用 TextEncoder 转成 UTF-8 字节再哈希，
//   否则前后端哈希永远不匹配，回环钩子2 兜底失效。
//
// 输入：channel|content（content 去首尾空白），输出：`mh:${8位hex}`。
// 注：contentHash 不含 conversationID / sender_id —— 同一文本在不同会话的哈希相同，
// 服务端 GetByMsgID 可跨会话去重 patrol 回声。不同客户发相同文本靠 sender_id 在 DB 查询层面区分。
export function contentHash(channel, conversationID, content) {
  const s = `${channel || ''}|${(content || '').trim()}`;
  // 统一为 UTF-8 字节，与 Go 端 []byte(s) 逐字节一致
  const bytes = typeof TextEncoder !== 'undefined'
    ? new TextEncoder().encode(s)
    : utf8EncodeFallback(s);
  let h = 0x811c9dc5;
  for (let i = 0; i < bytes.length; i++) {
    h ^= bytes[i];
    h = Math.imul(h, 0x01000193);
  }
  return 'mh:' + (h >>> 0).toString(16).padStart(8, '0');
}

// 无 TextEncoder 环境（极旧运行时）的 UTF-8 编码兜底
function utf8EncodeFallback(s) {
  const out = [];
  for (let i = 0; i < s.length; i++) {
    let c = s.charCodeAt(i);
    if (c < 0x80) out.push(c);
    else if (c < 0x800) {
      out.push(0xc0 | (c >> 6), 0x80 | (c & 0x3f));
    } else if (c >= 0xd800 && c <= 0xdbff) {
      const c2 = s.charCodeAt(++i);
      c = 0x10000 + ((c & 0x3ff) << 10) + (c2 & 0x3ff);
      out.push(0xf0 | (c >> 18), 0x80 | ((c >> 12) & 0x3f), 0x80 | ((c >> 6) & 0x3f), 0x80 | (c & 0x3f));
    } else {
      out.push(0xe0 | (c >> 12), 0x80 | ((c >> 6) & 0x3f), 0x80 | (c & 0x3f));
    }
  }
  return out;
}

// 解析下行回复（统一模型，字段名与服务端 UnifiedReply 完全一致）
// truncated 字段（2026-08-05 审计 P0 修复）：服务端截断 4KB 后置 true，
// 扩展可据此在 UI 显示"消息被截断"提示，避免用户看到半截消息不知情。
export function parseUnifiedReply(reply) {
  return {
    channel: reply.channel,
    account_id: reply.account_id,
    conversation_id: reply.conversation_id,
    msg_type: reply.msg_type || 'text',
    content: reply.content || '',
    media_url: reply.media_url || '',
    reply_to_event_id: reply.reply_to_event_id || '',
    truncated: reply.truncated === true,
  };
}
