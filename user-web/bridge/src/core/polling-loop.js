// Bridge HTTP Polling Loop (2026-08-05 架构重构)
//
// 替代 background/index.js 中的 WS 注册表 + registry.js 维护的"每渠道一条长连接"模式。
// 改为：每秒巡检一次会话列表 → 抓多轮消息 → 一次性 postIngest 上报。
//
// 工作流程（每 1 秒一轮）：
//   1. background 调度：getActiveAccount() → 当前 active (channel, account)
//   2. 调该渠道适配器的 getConversationList() 拿所有会话
//   3. 对每个会话：
//      a) 适配器.openConversation(conversationId) → 切换到该会话（保证消息列表可见）
//      b) 适配器.getMessages() → 抓所有可见消息（含 sender_type / content / timestamp）
//      c) 调 postIngest(...) 上报到 user-server
//      d) 拿到 outbound_replies → 调适配器.sendText(content) 发回网页
//   4. 空闲会话不发请求（减少无效网络）
//
// 优势：
//   - 0 长连接：MV3 SW 冻结 / 30s 回收不再影响
//   - 0 重连状态机：fetch 失败由 http-ingest.fetchWithRetry 自动退避
//   - 0 zombie 检测：HTTP 无状态
//   - 0 OOM：HTTP 用过即释放
//   - 0 同步复杂度：扩展的会话列表和已上报消息靠 content script 自管理
import { createLogger } from './logger.js';
import { postIngest, HTTP_INGEST_DEFAULTS } from './http-ingest.js';
import { DEFAULT_USER_SERVER } from './constants.js';

const log = createLogger('polling', 'bridge');

// 巡检周期：1000ms（用户诉求"1秒钟一个"）
const POLL_INTERVAL_MS = 1000;
// 单次轮询每个会话最多抓的消息数（防单次过多撑爆 body）
const MAX_MESSAGES_PER_CONVERSATION = 100;
// 同一会话两次上报最小间隔（避免对同一会话高频重复抓）
const MIN_PER_CONVERSATION_INTERVAL_MS = 800;
// 长轮询等待时间（每次 ingest 请求服务端最多等这么久拿 AI 回复）
const LONG_POLL_TIMEOUT_MS = HTTP_INGEST_DEFAULTS.longPollTimeoutMs;

class PollingLoop {
  constructor({ config, getAdapter, dispatchOutbound, getConfig, retryOpts }) {
    this.config = config;
    this.getAdapter = getAdapter; // (channel) => adapter | null
    this.dispatchOutbound = dispatchOutbound; // ({channel, account, conversation_id, content, msg_type, reply_to_event_id}) => boolean
    this.getConfig = getConfig || (() => this.config);
    // 透传 postIngest 的重试参数：生产不传，走 HTTP_INGEST_DEFAULTS；
    // 测试可显式覆盖 maxRetries=0 立即失败、retryBaseMs=1 跳过退避
    this.retryOpts = retryOpts || null;
    this._timer = null;
    this._running = false;
    this._inFlight = false;
    // 同一会话上次上报时间戳
    this._lastPollByConv = new Map(); // key: channel:account:conversation_id → ts ms
    // 已上报的 event_id 集合（防止重复抓+上报）
    this._seenEventIds = new Map(); // key: channel:account:conversation_id → Set<event_id>
    // 最近一轮已上报但 AI 未回（或网络失败）的事件 ID；用于下一轮重试或丢弃
    this._pendingReplyFor = new Map(); // key: event_id → { channel, account, conversation_id, content, sender_type, ts }
  }

  start() {
    if (this._timer) return;
    this._running = true;
    this._timer = setInterval(() => this._tickSafe(), POLL_INTERVAL_MS);
    log.info('巡检已启动：每 ' + POLL_INTERVAL_MS + 'ms 一轮');
  }

  stop() {
    this._running = false;
    if (this._timer) clearInterval(this._timer);
    this._timer = null;
    log.info('巡检已停止');
  }

  async _tickSafe() {
    if (!this._running) return;
    if (this._inFlight) return; // 上一轮未完成则跳过
    this._inFlight = true;
    try {
      await this._tick();
    } catch (e) {
      log.error('巡检 tick 失败', e);
    } finally {
      this._inFlight = false;
    }
  }

  async _tick() {
    const cfg = this.getConfig();
    if (!cfg) {
      log.warn('无配置，跳过本轮');
      return;
    }
    const serverUrl = cfg.serverUrl || DEFAULT_USER_SERVER.baseUrl;
    const token = cfg.token || '';
    // 当前活跃 (channel, account) 来自 background 的 metaStore
    const meta = cfg.active || null;
    if (!meta || !meta.channel || !meta.accountId) {
      return; // 无活动账号：本轮静默
    }
    const adapter = this.getAdapter(meta.channel);
    if (!adapter || typeof adapter.getConversationList !== 'function') {
      return; // 该渠道无适配器：本轮静默
    }
    // 取会话列表
    const convs = await safeCall(adapter, 'getConversationList', []);
    if (!convs || !convs.length) {
      return;
    }
    for (const conv of convs) {
      if (!conv || !conv.id) continue;
      const convKey = `${meta.channel}:${meta.accountId}:${conv.id}`;
      const now = Date.now();
      const last = this._lastPollByConv.get(convKey) || 0;
      if (now - last < MIN_PER_CONVERSATION_INTERVAL_MS) {
        continue; // 该会话本轮已抓过
      }
      this._lastPollByConv.set(convKey, now);
      // 切换到该会话（异步：openConversation 在大多数适配器是同步 click）
      try { await safeCall(adapter, 'openConversation', null, conv.id); } catch (_) { /* noop */ }
      // 给 DOM 一点时间渲染消息列表
      await sleep(120);
      // 抓消息
      const messages = await safeCall(adapter, 'getMessages', []);
      if (!messages || !messages.length) continue;
      // 过滤已上报
      const seenSet = this._seenEventIds.get(convKey) || new Set();
      const fresh = messages.filter((m) => m && m.message_id && !seenSet.has(m.message_id)).slice(0, MAX_MESSAGES_PER_CONVERSATION);
      if (!fresh.length) continue;
      // 标记已上报（即便后端去重，前端也避免重复抓+发）
      for (const m of fresh) seenSet.add(m.message_id);
      this._seenEventIds.set(convKey, seenSet);
      // 构造 body
      const body = {
        v: 2,
        channel: meta.channel,
        account_id: meta.accountId,
        conversation_id: conv.id,
        account_name: meta.accountName || '',
        agent_id: meta.agentId || 0,
        messages: fresh.map((m) => ({
          event_id: m.message_id,
          channel: meta.channel,
          account_id: meta.accountId,
          conversation_id: conv.id,
          sender_id: m.sender_id || conv.id,
          sender_name: m.sender_name || '',
          sender_type: m.sender_type || 'customer',
          msg_type: m.msg_type || 'text',
          content: m.text || '',
          media_url: m.media_url || '',
          timestamp: m.timestamp || Date.now(),
          is_group: !!m.is_group,
          group_id: m.group_id || '',
          group_name: m.group_name || '',
        })),
        expect_reply: true,
        timeout_ms: LONG_POLL_TIMEOUT_MS,
      };
      try {
        const resp = await postIngest(
          { serverUrl, channel: meta.channel, accountId: meta.accountId, conversationId: conv.id, token },
          body,
          {
            label: `[巡检 ingest ${meta.channel}:${conv.id}]`,
            timeoutMs: LONG_POLL_TIMEOUT_MS + 5000,
            ...(this.retryOpts || {}),
          }
        );
        if (resp && resp.outbound_replies && resp.outbound_replies.length) {
          for (const reply of resp.outbound_replies) {
            if (this.dispatchOutbound) {
              const ok = this.dispatchOutbound({
                channel: reply.channel || meta.channel,
                account_id: reply.account_id || meta.accountId,
                conversation_id: reply.conversation_id || conv.id,
                content: reply.content || '',
                msg_type: reply.msg_type || 'text',
                reply_to_event_id: reply.reply_to_event_id || '',
              });
              if (ok) {
                log.info('巡检收到 AI 回复并 dispatch', {
                  conversation_id: conv.id,
                  reply_to_event_id: reply.reply_to_event_id,
                  content_preview: (reply.content || '').slice(0, 60),
                });
              }
            }
          }
        }
      } catch (e) {
        // ingest 失败：把本轮 fresh 从 seen 中移除（下一轮重试）
        for (const m of fresh) seenSet.delete(m.message_id);
        log.warn('巡检 ingest 失败，下轮重试', { convKey, error: String(e && e.message || e) });
      }
    }
  }
}

async function safeCall(obj, method, fallback, ...args) {
  try {
    if (typeof obj[method] === 'function') {
      const r = obj[method](...args);
      if (r && typeof r.then === 'function') return await r;
      return r;
    }
  } catch (_) { /* 适配器方法不存在或抛错 */ }
  return fallback;
}

function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms));
}

export { PollingLoop, POLL_INTERVAL_MS, MAX_MESSAGES_PER_CONVERSATION };
