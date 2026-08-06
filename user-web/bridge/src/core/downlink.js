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
  // 下发转发超时（实现 BRIDGE_THREE_CHANNEL.sendOutboundTimeoutMs）：sendOutbound 卡死时
  // 不应永久阻塞整个轮询，超时则视为失败，下个周期重试。
  const sendTimeoutMs = options && options.sendOutboundTimeoutMs
    ? options.sendOutboundTimeoutMs
    : BRIDGE_THREE_CHANNEL.sendOutboundTimeoutMs;
  const outboxBatchSize = options && options.outboxBatchSize
    ? options.outboxBatchSize
    : BRIDGE_THREE_CHANNEL.outboxBatchSize;
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
    { label: `[下行 outbox] ${channel}`, batchSize: outboxBatchSize }
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
        ok = await withTimeout(
          sendOutbound(safeContent, m.conversation_id, { viaAdapter: channel }),
          sendTimeoutMs,
          `sendOutbound(${channel})`
        );
      } else {
        log.warn('pollDownlink: 未提供 sendOutbound，跳过下发', channel);
      }
    } catch (e) {
      ok = false;
    }

    // 关键顺序（2026-08-07 修复消息丢失边界）：
    //   先转发成功 → 再 ack 服务端 → 最后才写入本地已发缓存。
    //   若 ack 失败：本地不缓存，服务端仍 status=pending，下个轮询周期会重新拉取并重试，
    //   不会"前端已去重但服务端未标记"导致该消息永久丢失。
    if (ok) {
      ackIds.push(m.msg_id);
    }
    // 失败：不缓存、不 ack → 下个轮询周期重试（服务端仍 status=pending）
  }

  // 批量确认 delivered（实现 BRIDGE_THREE_CHANNEL.ackFlushIntervalMs 的语义：合并多次下发一次性 ack）
  if (ackIds.length) {
    const ackRes = await ackOutbox(
      { serverUrl, channel, accountId, token },
      ackIds,
      { label: `[下行 ack] ${channel}` }
    );
    // 仅当 ack 成功，才把这批 msg_id 写入本地已发缓存（防重复下发）；
    // ack 失败 → 不缓存 → 下个周期重试。
    if (ackRes && ackRes.status === 'ok') {
      for (const id of ackIds) cache.add(id);
    } else {
      log.warn(`下行 ack 失败，保留重试`, { channel, count: ackIds.length });
    }
  }
  await cache.flush();
}

// withTimeout 给 promise 加超时保护（不修改原 promise，仅 race）。
async function withTimeout(promise, ms, label) {
  if (!ms || ms <= 0) return promise;
  let timer = null;
  const timeout = new Promise((_, rej) => {
    timer = setTimeout(() => rej(new Error(`${label} timeout after ${ms}ms`)), ms);
  });
  try {
    return await Promise.race([promise, timeout]);
  } finally {
    if (timer) clearTimeout(timer);
  }
}
