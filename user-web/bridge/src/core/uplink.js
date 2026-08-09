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
import { contentHash } from './types.js';

// 前端完成消息 hash（稳定幂等）：同一消息产出同一 id。
// 与服务端 ContentHashMsgID 严格一致（channel|conversationID|trim(content)，FNV-1a 32 位，带 mh: 前缀），
// 作为回环去重钩子2（GetByMsgID）的兜底依据。
export function computeMsgID({ channel, accountId, conversationId, content }) {
  return contentHash(channel, conversationId, content);
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
    // 上报 ack 客户端闭环：持久化已确认(event_id)集合，刷新/重载后不再重复上行已确认消息。
    // 服务端仍是权威去重（钩子2 + 中间件），此为前端二次防御，降低冗余请求；降级：storage 不可用时退化为不持久化。
    this._confirmed = new Set();
    this._confirmedLoaded = false;
  }

  // _loadConfirmed 从 chrome.storage.local 载入已确认集合（幂等 ack 闭环，跨刷新保留）。
  async _loadConfirmed() {
    if (this._confirmedLoaded) return;
    try {
      if (typeof chrome !== 'undefined' && chrome.storage && chrome.storage.local) {
        const r = await chrome.storage.local.get('mtk_uplink_confirmed');
        const arr = (r && r.mtk_uplink_confirmed) || [];
        if (Array.isArray(arr)) this._confirmed = new Set(arr);
      }
    } catch (_) {
      // 降级：保留内存空集合，行为同旧版（仅依赖服务端去重）
    }
    this._confirmedLoaded = true;
  }

  // _saveConfirmed 持久化已确认集合（限制规模防爆增）。
  async _saveConfirmed() {
    try {
      if (typeof chrome !== 'undefined' && chrome.storage && chrome.storage.local) {
        if (this._confirmed.size > 5000) {
          this._confirmed = new Set([...this._confirmed].slice(-2000));
        }
        await chrome.storage.local.set({ mtk_uplink_confirmed: [...this._confirmed] });
      }
    } catch (_) {
      // 降级：忽略持久化失败
    }
  }

  // _markConfirmedFromResponse 上报 ack：把服务端确认(accepted/duplicate)的 event_id 记入已确认集合。
  _markConfirmedFromResponse(resp, items) {
    if (!resp || !Array.isArray(resp.ingested)) return;
    const byEvent = new Map();
    for (const it of items) {
      if (it.event_id) byEvent.set(it.event_id, true);
    }
    for (const r of resp.ingested) {
      if (r && (r.accepted || r.duplicate) && r.event_id && byEvent.has(r.event_id)) {
        this._confirmed.add(r.event_id);
      }
    }
    if (this._confirmed.size) this._saveConfirmed();
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
        content: message.content,
      });
    }
    // 回环去重兜底：无论 event_id 来源（DOM id / c:text / 兜底 hash），都附带与后端
    // ContentHashMsgID 同源的 content_hash。后端 GetByMsgID 命中 content_hash 即幂等跳过
    // （钩子2），作为 isPlatformOutboundEcho 内容匹配之外的第二道防线。
    if (!message.content_hash) {
      message.content_hash = contentHash(
        message.channel || this.channel,
        conversationId,
        message.content
      );
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
    // 上报 ack 闭环：载入已确认集合后过滤掉服务端已确认(event_id)的消息，避免重复上行。
    await this._loadConfirmed();
    const pending = items.filter((m) => !(m.event_id && this._confirmed.has(m.event_id)));
    if (!pending.length) return;
    const sample = pending[0];
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
      messages: pending.map((m) => ({
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
        content_hash: m.content_hash || '',
      })),
      timeout_ms: HTTP_INGEST_DEFAULTS.longPollTimeoutMs,
    };
    const label = `[上行 ingest 合并 ×${pending.length}]`;
    try {
      const resp = await postIngest(
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
      // 上报 ack：把服务端确认(accepted/duplicate)的 event_id 记入已确认集合（闭环，避免重复上行）
      this._markConfirmedFromResponse(resp, pending);
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
