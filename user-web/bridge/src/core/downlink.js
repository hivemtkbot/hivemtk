// Bridge 下发轮询 + 状态确认（通道C·下发 / 通道B·状态，2026-08-06 架构重构）
//
// 设计目标（用户诉求）：
//   - 前端独立轮询 GET /api/bridge/outbox 拉取待发消息（多条），转发到对应渠道；
//   - 转发成功后通过 POST /api/bridge/outbox/ack 上报 delivered，服务端标记数据库状态；
//   - 本地已发缓存（持久化到 chrome.storage.local）：严重消息绝不允许重复发给用户；
//   - 服务端下发队列去重：仅 status=pending 出站消息入队，确认 delivered 后排除。
//
// 与通道A·上报（core/uplink.js）完全独立：上报即时返回，回复经下发队列异步拉取。
import { getOutbox, ackOutbox } from './http-ingest.js';
import { sanitizeForDisplay } from './sanitize.js';
import { BRIDGE_THREE_CHANNEL } from './constants.js';
import { createLogger } from './logger.js';

const log = createLogger('downlink');

// 本地已发缓存：防刷新 / SW 冻结重开后重复下发同一条消息给用户。
// 按 channel 隔离，持久化到 chrome.storage.local。
class SentCache {
  constructor(channel) {
    this.channel = channel;
    this.key = `bridge_sent_${channel}`;
    this.mem = new Set();
    this.dirty = false;
    this.loaded = false;
  }

  async load() {
    if (this.loaded) return;
    try {
      const v = await chrome.storage.local.get([this.key]);
      const arr = v && v[this.key];
      if (Array.isArray(arr)) this.mem = new Set(arr);
    } catch (_) {
      /* storage 不可用时退化为内存缓存 */
    }
    this.loaded = true;
  }

  has(id) {
    return this.mem.has(id);
  }

  add(id) {
    this.mem.add(id);
    this.dirty = true;
    if (this.mem.size > BRIDGE_THREE_CHANNEL.sentCacheMax) {
      const arr = [...this.mem];
      this.mem = new Set(arr.slice(arr.length - BRIDGE_THREE_CHANNEL.sentCacheMax));
    }
  }

  async flush() {
    if (!this.dirty) return;
    try {
      await chrome.storage.local.set({ [this.key]: [...this.mem] });
      this.dirty = false;
    } catch (_) {
      /* 持久化失败不影响内存去重 */
    }
  }
}

const caches = {};
function getCache(channel) {
  if (!caches[channel]) caches[channel] = new SentCache(channel);
  return caches[channel];
}

// 初始化：预加载各渠道已发缓存（在轮询开始前调用一次）
export async function initDownlink(channels) {
  for (const ch of channels) {
    await getCache(ch).load();
  }
}

// pollDownlink 通道C·下发轮询 + 通道B·状态确认。
// 拉取本渠道待发消息 → 逐条转发对应网页会话 → 成功后批量 ack delivered。
export async function pollDownlink(channel, accountId, getConfig, options = {}) {
  const sendOutbound = options && typeof options.sendOutbound === 'function' ? options.sendOutbound : null;
  const cache = getCache(channel);
  await cache.load();
  let cfg = {};
  try {
    cfg = (await getConfig()) || {};
  } catch (_) {
    cfg = {};
  }
  const serverUrl = cfg.serverUrl || '';
  const token = cfg.token || '';
  if (!serverUrl) return;

  const res = await getOutbox(
    { serverUrl, channel, accountId, token },
    { label: `[下行 outbox] ${channel}` }
  );
  const messages = (res && res.messages) || [];
  const ackIds = [];

  for (const m of messages) {
    if (!m || !m.msg_id) continue;
    if (cache.has(m.msg_id)) continue; // 已发过，绝不重复
    const raw = m.content || '';
    if (!raw) continue; // 空内容不下发（避免占位空消息打给用户）
    // XSS 防护：净化内容（控制长度、去控制字符）
    const safeContent = sanitizeForDisplay ? sanitizeForDisplay(raw) : raw;

    let ok = false;
    try {
      if (sendOutbound) {
        ok = await sendOutbound(safeContent, m.conversation_id, { viaAdapter: channel });
      } else {
        log.warn('pollDownlink: 未提供 sendOutbound，跳过下发', channel);
      }
    } catch (e) {
      ok = false;
    }

    if (ok) {
      cache.add(m.msg_id);
      ackIds.push(m.msg_id);
    }
    // 失败：不缓存、不 ack → 下个轮询周期重试（服务端仍 status=pending）
  }

  if (ackIds.length) {
    await ackOutbox(
      { serverUrl, channel, accountId, token },
      ackIds,
      { label: `[下行 ack] ${channel}` }
    );
  }
  await cache.flush();
}
