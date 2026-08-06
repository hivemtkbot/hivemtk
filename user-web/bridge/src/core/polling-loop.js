// Bridge HTTP Polling Loop (2026-08-06 架构重构：下发三通道分离)
//
// 替代 background/index.js 中的 WS 注册表 + registry.js 维护的"每渠道一条长连接"模式。
// 改为三个互相独立的通道：
//   通道A·上报:  Uplink（父层统一上报）→ POST /api/bridge/ingest（即时返回，消息 hash 前端完成）
//   通道B·状态:  ackOutbox（由 downlink 调用）→ POST /api/bridge/outbox/ack
//   通道C·下发:  downlink.pollDownlink → GET /api/bridge/outbox 独立轮询拉取待发消息
//
// 巡检定时任务（要求⑤）：每 3 秒一轮遍历新会话列表，逐会话切换随机 1-2 秒，
//   把消息送入统一上报通道（Uplink）。下发与上报完全解耦，互不影响、各自并发。
//
// 优势（相对旧版）：
//   - 上报即时返回：不再为等 AI 回复挂 500s 长连接（frp 抖动即丢消息）
//   - 下发独立轮询 + 本地已发缓存：不会重复下发、有 ack 同步确认
//   - 0 长连接 / 0 重连状态机 / 0 zombie 检测：HTTP 无状态
import { createLogger } from './logger.js';
import { BRIDGE_THREE_CHANNEL } from './constants.js';
import { Uplink } from './uplink.js';
import { pollDownlink, initDownlink } from './downlink.js';

const log = createLogger('polling', 'bridge');

// 单次巡检每个会话最多抓的消息数（防单次过多撑爆 body）
const MAX_MESSAGES_PER_CONVERSATION = 100;

class PollingLoop {
  constructor({ config, getAdapter, getConfig, getMeta, retryOpts, channels }) {
    this.config = config;
    this.getAdapter = getAdapter; // (channel) => adapter | null
    this.getConfig = getConfig || (() => this.config);
    // getMeta：返回 { accountId, conversationId }（渠道适配器提供，如 adapter.getAccountId()）
    // 用于统一上报 / 下发轮询的账号归属，确保 ingest 与 outbox 查询账号一致。
    this.getMeta = getMeta || (() => ({}));
    // 透传 postIngest 的重试参数：生产不传，走 HTTP_INGEST_DEFAULTS；
    // 测试可显式覆盖 maxRetries=0 立即失败、retryBaseMs=1 跳过退避
    this.retryOpts = retryOpts || null;
    this.channels = channels || [];
    // 每个渠道一个 Uplink 实例（统一上报父层，三通道相互独立）
    this.uplinks = new Map();
    for (const ch of this.channels) {
      this.uplinks.set(ch, new Uplink({ channel: ch, getConfig: () => this.getConfig(), retryOpts: this.retryOpts }));
    }
    this._patrolTimer = null;
    this._downlinkTimer = null;
    this._running = false;
    this._patrolInFlight = false;
    this._downlinkInFlight = false;
  }

  start() {
    if (this._running) return;
    this._running = true;
    // 通道A·上报 + 巡检：每 patrolIntervalMs 一轮
    this._patrolTimer = setInterval(
      () => this._patrolSafe(),
      BRIDGE_THREE_CHANNEL.patrolIntervalMs
    );
    // 通道C·下发轮询：每 outboxPollIntervalMs 一轮（与上报完全独立）
    this._downlinkTimer = setInterval(
      () => this._downlinkSafe(),
      BRIDGE_THREE_CHANNEL.outboxPollIntervalMs
    );
    initDownlink(this.channels);
    log.info(
      `巡检+下发已启动：上报巡检每 ${BRIDGE_THREE_CHANNEL.patrolIntervalMs}ms，下发轮询每 ${BRIDGE_THREE_CHANNEL.outboxPollIntervalMs}ms`
    );
  }

  stop() {
    this._running = false;
    if (this._patrolTimer) clearInterval(this._patrolTimer);
    if (this._downlinkTimer) clearInterval(this._downlinkTimer);
    this._patrolTimer = null;
    this._downlinkTimer = null;
    log.info('巡检+下发已停止');
  }

  async _patrolSafe() {
    if (!this._running || this._patrolInFlight) return;
    this._patrolInFlight = true;
    try {
      await this._patrol();
    } catch (e) {
      log.error('巡检 tick 失败', e);
    } finally {
      this._patrolInFlight = false;
    }
  }

  async _patrol() {
    const cfg = await this.getConfig();
    if (!cfg) {
      log.warn('无配置，跳过本轮巡检');
      return;
    }
    // 配置（serverUrl）是上报入口的唯一真相源：缺失时静默跳过，
    // 绝不盲目回退到 localhost:8204（那是本地 user-server，不是生产穿透地址）。
    const serverUrl = cfg.serverUrl;
    if (!serverUrl) {
      log.warn('无 serverUrl，跳过本轮巡检');
      return;
    }
    const meta = this.getMeta ? this.getMeta() : {};
    const accountId = (meta && meta.accountId) || (cfg.active && cfg.active.accountId) || 'default';
    const channel = cfg.active && cfg.active.channel ? cfg.active.channel : this.channels[0];
    if (!channel) return;
    const adapter = this.getAdapter(channel);
    if (!adapter || typeof adapter.getConversationList !== 'function') {
      return; // 该渠道无适配器：本轮静默
    }
    const uplink = this.uplinks.get(channel);
    if (!uplink) return;

    // 取会话列表
    const convs = await safeCall(adapter, 'getConversationList', []);
    if (!convs || !convs.length) return;

    for (const conv of convs) {
      if (!conv || !conv.id) continue;
      // 切换到该会话（保证消息列表可见）
      try {
        await safeCall(adapter, 'openConversation', null, conv.id);
      } catch (_) {
        /* noop */
      }
      // 每个会话列表切换随机等待 1-2 秒（要求⑤：给 SPA 足够渲染时间）
      const wait = randInt(
        BRIDGE_THREE_CHANNEL.patrolSwitchMinMs,
        BRIDGE_THREE_CHANNEL.patrolSwitchMaxMs
      );
      await sleep(wait);
      // 抓消息
      const messages = await safeCall(adapter, 'getMessages', []);
      if (!messages || !messages.length) continue;
      // 纯桥接：不做前端去重，所有可见消息统一推入上报队列（内容去重交给后端）
      const fresh = messages.slice(0, MAX_MESSAGES_PER_CONVERSATION);
      for (const m of fresh) {
        uplink.enqueue({
          channel,
          account_id: accountId,
          conversation_id: conv.id,
          event_id: m.message_id,
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
        });
      }
    }
    // 本轮巡检结束，统一 flush 上报（合并窗口 + 退避重试）
    await uplink.flushAll();
  }

  async _downlinkSafe() {
    if (!this._running || this._downlinkInFlight) return;
    this._downlinkInFlight = true;
    try {
      await this._downlink();
    } catch (e) {
      log.error('下发轮询失败', e);
    } finally {
      this._downlinkInFlight = false;
    }
  }

  async _downlink() {
    const cfg = await this.getConfig();
    if (!cfg) return;
    const meta = this.getMeta ? this.getMeta() : {};
    const accountId = (meta && meta.accountId) || (cfg.active && cfg.active.accountId) || 'default';
    // 通道C·下发轮询：逐渠道独立拉取待发消息并转发（与上报互不阻塞）
    for (const ch of this.channels) {
      try {
        // 适配器由 getAdapter 解析（与巡检同一来源），注入 sendOutbound 供下发转发
        const adapter = this.getAdapter ? this.getAdapter(ch) : null;
        const sendOutbound = adapter && typeof adapter.sendOutbound === 'function'
          ? (text, convId, opts) => adapter.sendOutbound(text, convId, opts)
          : undefined;
        await pollDownlink(ch, accountId, () => this.getConfig(), { sendOutbound });
      } catch (e) {
        log.warn(`下发轮询失败 channel=${ch}`, { error: String(e && e.message || e) });
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
  } catch (_) {
    /* 适配器方法不存在或抛错 */
  }
  return fallback;
}

function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms));
}

function randInt(min, max) {
  return Math.floor(min + Math.random() * (max - min + 1));
}

export { PollingLoop, MAX_MESSAGES_PER_CONVERSATION };
