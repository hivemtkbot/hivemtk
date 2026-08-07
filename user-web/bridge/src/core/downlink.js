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
import { BRIDGE_THREE_CHANNEL, RATE_LIMIT_DEFAULTS } from './constants.js';
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
// 拉取本渠道待发消息 → 按 conversation_id 分组并行转发到对应网页会话 → 成功后批量 ack delivered。
//
// 2026-08-07 修复（用户诉求 ①②③）：
//   ① 下发转发不应该串行——回复谁的消息就应该在谁的消息输入框。
//      旧版 for...of 串行处理：第 1 条发送成功 markSent → lastGlobalSendAt 立刻更新 → 第 2 条
//      tryAcquire 命中 minIntervalMs(1500ms) → 整批 break → 第二个会话永远不发。
//      改为按 conversation_id 分组并行：每个会话独立 tryAcquire/markSent/rate-limit，
//      会话 A 限速拦截不影响会话 B；同时同一会话内按原顺序串行（保留单会话限速语义）。
//   ② 下发转发应该即时——并行处理 + 会话内串行减少总等待时间。
//   ③ 每次下发转发消息应该清空输入框内容——见 xhs.js sendText fillContentEditable 调用前先清空。
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

  // —— 按 conversation_id 分组（同会话内保留原顺序以维持单会话限速语义）——
  const groups = new Map(); // conversation_id -> [{ msg, sanitized }]
  for (const m of messages) {
    if (!m || !m.msg_id) continue;
    if (cache.has(m.msg_id)) continue; // 已发过，绝不重复
    const raw = m.content || '';
    if (!raw) continue; // 空内容不下发（避免占位空消息打给用户）
    // XSS 防护：净化内容（控制长度、去控制字符）
    const safeContent = sanitizeForDisplay ? sanitizeForDisplay(raw) : raw;
    const convId = m.conversation_id || '_unknown_';
    if (!groups.has(convId)) groups.set(convId, []);
    groups.get(convId).push({ msg: m, sanitized: safeContent });
  }

  // —— 按 conversation_id 串行转发（核心修复：杜绝跨会话污染）——
  // 背景：桥接内容脚本运行在单个 SPA 页（小红书/抖音聊天页），多个会话共用同一页面 DOM。
  //   sendOutbound 内部会 openConversation(convId) 切换右侧会话再 fillAndSend。若多个会话
  //   并行 openConversation，页面导航会相互竞争，导致 A 会话的文案被填进 B 会话的输入框——
  //   即"下发给 A 的消息却发给了 B/所有人"。故必须按会话串行，保证"切会话→填发→完成"原子，
  //   单会话内也串行（同会话限速语义）。
  // 限速处理：全局 minInterval 会让"紧接其后的另一会话"首条被 rateLimited。此时就地等待后
  //   重试（有限次），而非 break 丢弃——既保留拟人限速，又确保每会话都最终下发（不丢消息）。
  const allAckIds = [];
  const MAX_RATE_RETRIES = 3; // 单条消息限速重试上限（防单会话卡死整轮）
  for (const [convId, group] of groups) {
    const sentIds = [];
    let convAbort = false; // 本会话因持续限速放弃剩余，全部留 pending 下轮再试
    for (const { msg, sanitized } of group) {
      if (convAbort) break;
      let result = null;
      try {
        if (sendOutbound) {
          result = await withTimeout(
            sendOutbound(sanitized, msg.conversation_id, { viaAdapter: channel }),
            sendTimeoutMs,
            `sendOutbound(${channel}:${convId})`
          );
        } else {
          log.warn('pollDownlink: 未提供 sendOutbound，跳过下发', channel);
        }
      } catch (e) {
        result = null;
      }
      const ok = !!(result && result.ok);
      const rateLimited = !!(result && result.rateLimited);
      const notFound = !!(result && result.notFound);
      if (ok) { sentIds.push(msg.msg_id); continue; }
      // 目标会话在页面左侧列表不存在：放弃本条（留 pending，下次巡检/换端可能拉到），不重试。
      if (notFound) {
        log.debug(`下行目标会话不存在，跳过并留 pending: ${convId}`, { msg_id: msg.msg_id });
        continue;
      }
      // 拟人限速（全局 minInterval / 会话冷却 / 账号桶）：就地等待后重试本条，有限次。
      if (rateLimited) {
        let delivered = false;
        for (let attempt = 0; attempt < MAX_RATE_RETRIES && !delivered; attempt++) {
          const waitMs =
            options.rateRetryWaitMs != null ? options.rateRetryWaitMs : rateRetryBaseMs();
          await sleep(waitMs);
          try {
            const r2 = await withTimeout(
              sendOutbound(sanitized, msg.conversation_id, { viaAdapter: channel }),
              sendTimeoutMs,
              `sendOutbound-retry(${channel}:${convId})`
            );
            if (r2 && r2.ok) { delivered = true; sentIds.push(msg.msg_id); }
            else if (r2 && r2.notFound) break;      // 目标会话不存在：停止重试本条
            else if (r2 && !r2.rateLimited) break;   // 其它失败（非限速）：停止重试本条
          } catch (e) { /* 超时等：受 MAX_RATE_RETRIES 约束继续重试 */ }
        }
        if (delivered) continue;
        // 重试耗尽仍受限：放弃本会话剩余消息（它们大概率也受限），全部留 pending 下轮再试。
        // 关键：不污染其它会话——因为已按会话串行，当前会话的导航竞争不会波及后续会话。
        log.warn(`下行会话持续限速，放弃本会话剩余并留 pending`, {
          channel, convId, pending: group.length - sentIds.length,
        });
        convAbort = true;
        break;
      }
    }
    // 关键顺序（2026-08-07 修复消息丢失边界）：
    //   先转发成功 → 再 ack 服务端 → 最后才写入本地已发缓存。
    //   若 ack 失败：本地不缓存，服务端仍 status=pending，下个轮询周期会重新拉取并重试，
    //   不会"前端已去重但服务端未标记"导致该消息永久丢失。
    if (sentIds.length) {
      const ackRes = await ackOutbox(
        { serverUrl, channel, accountId, token },
        sentIds,
        { label: `[下行 ack] ${channel}:${convId}` }
      );
      if (ackRes && ackRes.status === 'ok') {
        for (const id of sentIds) cache.add(id);
        allAckIds.push(...sentIds);
      } else {
        log.warn(`下行单会话 ack 失败，保留重试`, { channel, convId, count: sentIds.length });
      }
    }
  }
  await cache.flush();
  if (allAckIds.length) log.info(`下行完成: ${channel} 共 ${allAckIds.length} 条已 ack`, { convs: groups.size });
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

// sleep：限速重试等待（拟人节奏，避免被识别为机器人）。
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// rateRetryBaseMs：单行限速重试的等待时长（对齐三层风控的拟人节奏）。
// 取全局最小间隔 + 随机抖动，使"紧接其后的另一会话"在等待后能通过 minInterval 校验。
function rateRetryBaseMs() {
  const { minIntervalMs, jitterMinMs, jitterMaxMs } = RATE_LIMIT_DEFAULTS;
  const jitter = jitterMinMs + Math.random() * Math.max(0, jitterMaxMs - jitterMinMs);
  return minIntervalMs + jitter;
}
