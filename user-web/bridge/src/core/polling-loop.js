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
    this.getAdapter = getAdapter; 
    this.getConfig = getConfig || (() => this.config);
    this.getMeta = getMeta || (() => ({}));
    this.retryOpts = retryOpts || null;
    this.channels = channels || [];
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
    this._patrolTimer = setInterval(
      () => this._patrolSafe(),
      BRIDGE_THREE_CHANNEL.patrolIntervalMs
    );
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
      return; 
    }
    const uplink = this.uplinks.get(channel);
    if (!uplink) return;

    // 取会话列表
    const convs = await safeCall(adapter, 'getConversationList', []);
    if (!convs || !convs.length) return;

    for (const conv of convs) {
      if (!conv || !conv.id) continue;
      try {
        await safeCall(adapter, 'openConversation', null, conv.id);
      } catch (_) {
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
    for (const ch of this.channels) {
      try {
        // 适配器由 getAdapter 解析（与巡检同一来源），注入 sendOutbound 供下发转发
        const adapter = this.getAdapter ? this.getAdapter(ch) : null;
        const sendOutbound = adapter && typeof adapter.sendOutbound === 'function'
          ? (text, convId, opts) => adapter.sendOutbound(text, convId, opts)
          : undefined;
        await pollDownlink(ch, accountId, () => this.getConfig(), {
          sendOutbound,
          sendOutboundTimeoutMs: BRIDGE_THREE_CHANNEL.sendOutboundTimeoutMs,
          outboxBatchSize: BRIDGE_THREE_CHANNEL.outboxBatchSize,
        });
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

