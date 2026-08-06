// Bridge 统一上报父层（通道A·上报，2026-08-06 架构重构）
//
// 设计目标（用户诉求）：
//   1. 顶部维持一个 HTTP 上报机制，4 个渠道（douyin/xhs/tiktok/xianyu）只负责把聊天内容
//      推入上报队列，不再各自维护散落的 postIngest。
//   2. 消息 hash（msg_id / event_id）在前端完成：后端以 event_id 作为去重主键
//      （message_hub.MsgID 唯一约束），因此 hash 必须在前端算好随消息上行。
//
// 三通道相互独立：
//   通道A·上报:  Uplink        → POST /api/bridge/ingest
//   通道B·状态:  ackOutbox      → POST /api/bridge/outbox/ack
//   通道C·下发:  getOutbox      → GET  /api/bridge/outbox   （见 core/downlink.js）
//
// 复用 http-ingest.postIngest（统一 URL 构造 + 日志 + 退避重试）。
import { postIngest, HTTP_INGEST_DEFAULTS } from './http-ingest.js';
import { DEFAULT_USER_SERVER, BRIDGE_THREE_CHANNEL } from './constants.js';

// 前端完成消息 hash（稳定幂等）：同一消息（同渠道/账号/会话/发送方/内容/时间）产出同一 id。
// 采用 FNV-1a（32 位），纯同步、无依赖，适合在 content script / SW 中高频调用。
export function computeMsgID({ channel, accountId, conversationId, senderType, content, ts }) {
  const s = [
    channel || '',
    accountId || '',
    conversationId || '',
    senderType || '',
    (content || '').slice(0, 2000),
    String(ts || 0),
  ].join('|');
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return 'b-' + (h >>> 0).toString(16).padStart(8, '0');
}

// Uplink：统一上报队列。所有渠道经 enqueue(message) 推入，按 (accountId|conversationId)
// 短窗口合并为一次 POST，显著降低冗余请求；消息 hash 在此兜底补全。
export class Uplink {
  constructor({ channel, getConfig, retryOpts } = {}) {
    this.channel = channel;
    this.getConfig = getConfig || (async () => ({}));
    // 透传 postIngest 的重试参数（测试可覆盖 maxRetries=0 立即失败）
    this.retryOpts = retryOpts || null;
    this.buffers = new Map(); // key -> { items: UnifiedMessage[], timer }
    this.mergeWindowMs = BRIDGE_THREE_CHANNEL.uplinkMergeWindowMs;
    this.maxBatch = BRIDGE_THREE_CHANNEL.uplinkMaxBatch;
  }

  // enqueue 推入一条消息（customer / self / agent 均走此统一入口）
  enqueue(message) {
    if (!message) return;
    const accountId = message.account_id || '';
    const conversationId = message.conversation_id || '';
    if (!accountId || !conversationId) return;
    // 消息 hash 前端完成：渠道已给 event_id 则沿用（最稳，来自 DOM data-message-id），否则兜底
    if (!message.event_id) {
      message.event_id = computeMsgID({
        channel: message.channel || this.channel,
        accountId,
        conversationId,
        senderType: message.sender_type,
        content: message.content,
        ts: message.timestamp,
      });
    }
    const key = `${accountId}|${conversationId}`;
    let buf = this.buffers.get(key);
    if (!buf) {
      buf = { items: [], timer: null };
      this.buffers.set(key, buf);
    }
    buf.items.push(message);
    if (buf.items.length >= this.maxBatch) {
      this._flush(key);
    } else if (!buf.timer) {
      buf.timer = setTimeout(() => this._flush(key), this.mergeWindowMs);
    }
  }

  async _flush(key) {
    const buf = this.buffers.get(key);
    if (!buf) return;
    this.buffers.delete(key); // 取走所有权，避免并发重复 flush
    if (buf.timer) {
      clearTimeout(buf.timer);
      buf.timer = null;
    }
    const items = buf.items;
    if (!items.length) return;
    const sample = items[0];
    let cfg = {};
    try {
      cfg = (await this.getConfig()) || {};
    } catch (_) {
      cfg = {};
    }
    const accountId = sample.account_id || '';
    const conversationId = sample.conversation_id || '';
    if (!accountId || !conversationId) return;
    const ch = sample.channel || this.channel;
    const body = {
      v: 2,
      channel: ch,
      account_id: accountId,
      conversation_id: conversationId,
      account_name: '',
      agent_id: 0,
      // 多条消息包成 messages[]：服务端按 msg_id 去重 + 逐条落库
      messages: items.map((m) => ({
        event_id: m.event_id || '',
        channel: m.channel || ch,
        account_id: accountId,
        conversation_id: conversationId,
        sender_id: m.sender_id || '',
        sender_name: m.sender_name || '',
        sender_type: m.sender_type || 'customer',
        msg_type: m.msg_type || 'text',
        content: m.content || '',
        media_url: m.media_url || '',
        timestamp: m.timestamp || Date.now(),
        is_group: !!m.is_group,
        group_id: m.group_id || '',
        group_name: m.group_name || '',
        history: Array.isArray(m.history) ? m.history : [],
      })),
      timeout_ms: HTTP_INGEST_DEFAULTS.longPollTimeoutMs,
    };
    const label = `[上行 ingest 合并 ×${items.length}]`;
    try {
      await postIngest(
        {
          serverUrl: cfg.serverUrl || DEFAULT_USER_SERVER.baseUrl,
          channel: ch,
          accountId,
          conversationId,
          token: cfg.token || '',
        },
        body,
        {
          label,
          timeoutMs: HTTP_INGEST_DEFAULTS.longPollTimeoutMs + 5000,
          ...(this.retryOpts || {}),
        }
      );
    } catch (e) {
      // 失败不抛：postIngest 自带退避重试；此处仅记录，避免污染调用栈
    }
  }

  // flushAll 强制清空所有缓冲（轮询周期末调用，确保低峰期也及时上行）
  async flushAll() {
    const keys = [...this.buffers.keys()];
    await Promise.all(keys.map((k) => this._flush(k)));
  }
}
