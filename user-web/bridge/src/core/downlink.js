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

// _pendingAckByChannel：按渠道维护"待重试 ack"的 msg_id 队列（2026-08-15 P0-9 升级为带退避的 Map）。
//
// 2026-08-14 修复（用户诉求 "不要兜底逻辑会导致垃圾数据" 的反例——这里必须"兜底"）：
//   sendOutbound 已成功 = 用户已收到。这条事实是"用户是否收到"的唯一权威。
//   ack 服务端只是后端记账的副作用——若 ack 失败（网络/服务暂时不可用），
//   必须 (a) 立刻写本地已发缓存防重发 (b) 记入此队列等下轮重发 ack。
//
// 2026-08-15 P0-9 升级（10/10 任务清单）：
//   - 由 Set 升级为 Map<msg_id, { attempts, firstSeenAt, lastTryAt, lastError }>
//   - 加入最大重试次数（MAX_ACK_RETRY_ATTEMPTS=10），超过则放弃重试但仍保留 cache
//     （用户已收到，后端状态机不一致无影响；避免永久重试导致队列膨胀）
//   - 加入指数退避（baseMs=1000 → 1s, 2s, 4s, 8s, 16s, 32s, 60s, 60s, 60s, 60s）
//   - 加入条目级 TTL（MAX_ACK_ENTRY_AGE_MS=24h），超过则清理（防永久残留）
const MAX_ACK_RETRY_ATTEMPTS = 10;
const MAX_PENDING_ACK_PER_CHANNEL = 1000;
const ACK_RETRY_BACKOFF_BASE_MS = 1000;     // 首次退避 1s
const ACK_RETRY_BACKOFF_CAP_MS = 60_000;    // 退避上限 60s
const MAX_ACK_ENTRY_AGE_MS = 24 * 60 * 60 * 1000; // 单条 msg_id 在 pendingAck 中最长存活 24h
const _pendingAckByChannel = {};
function this_pendingAckFor(channel) {
  if (!_pendingAckByChannel[channel]) _pendingAckByChannel[channel] = new Map();
  const m = _pendingAckByChannel[channel];
  if (m.size > MAX_PENDING_ACK_PER_CHANNEL) {
    // 容量保护：按 firstSeenAt 升序淘汰最早的条目（FIFO）
    const sorted = [...m.entries()].sort((a, b) => a[1].firstSeenAt - b[1].firstSeenAt);
    const evictCount = m.size - MAX_PENDING_ACK_PER_CHANNEL;
    for (let i = 0; i < evictCount; i++) m.delete(sorted[i][0]);
  }
  return m;
}

// ackRetryBackoffMs 计算指定 attempt 次数（从 1 开始）的退避时长（毫秒）。
// 第 1 次：baseMs=1000；第 2 次：2000；第 3 次：4000；...；第 7 次及之后：capMs=60000。
export function ackRetryBackoffMs(attempt) {
  if (attempt < 1) return ACK_RETRY_BACKOFF_BASE_MS;
  const ms = ACK_RETRY_BACKOFF_BASE_MS * Math.pow(2, attempt - 1);
  return Math.min(ms, ACK_RETRY_BACKOFF_CAP_MS);
}

// addPendingAck 添加 msg_id 到重试队列（首次入队时设置 firstSeenAt=now, attempts=0）。
export function addPendingAck(channel, msgId, lastError) {
  const m = this_pendingAckFor(channel);
  if (m.has(msgId)) {
    const entry = m.get(msgId);
    entry.lastTryAt = Date.now();
    if (lastError) entry.lastError = lastError;
    return entry;
  }
  const now = Date.now();
  const entry = { attempts: 0, firstSeenAt: now, lastTryAt: 0, lastError: lastError || '' };
  m.set(msgId, entry);
  // 容量保护：超过上限时按 firstSeenAt 升序淘汰最早的条目（FIFO）
  if (m.size > MAX_PENDING_ACK_PER_CHANNEL) {
    const sorted = [...m.entries()].sort((a, b) => a[1].firstSeenAt - b[1].firstSeenAt);
    const evictCount = m.size - MAX_PENDING_ACK_PER_CHANNEL;
    for (let i = 0; i < evictCount; i++) m.delete(sorted[i][0]);
  }
  return entry;
}

// claimDuePendingAck 提取本轮可重试的 msg_id（已达退避时长 且 未超最大次数 且 未超 TTL）。
export function claimDuePendingAck(channel) {
  const m = this_pendingAckFor(channel);
  const now = Date.now();
  const due = [];
  for (const [msgId, entry] of m) {
    if (now - entry.firstSeenAt > MAX_ACK_ENTRY_AGE_MS) {
      m.delete(msgId);
      continue;
    }
    if (entry.attempts >= MAX_ACK_RETRY_ATTEMPTS) {
      m.delete(msgId);
      log.warn(`下行 ack 达到最大重试次数 ${MAX_ACK_RETRY_ATTEMPTS}，放弃重试（cache 保留防内容重发）`,
        { channel, msg_id: msgId, attempts: entry.attempts, last_error: entry.lastError });
      continue;
    }
    const backoff = ackRetryBackoffMs(entry.attempts + 1);
    if (entry.lastTryAt && now - entry.lastTryAt < backoff) continue;
    due.push({ msgId, entry });
  }
  return due;
}

// markPendingAckTried 在重试后更新 attempts/lastTryAt。
export function markPendingAckTried(channel, msgId, success, error) {
  const m = this_pendingAckFor(channel);
  if (success) {
    m.delete(msgId);
    return;
  }
  const entry = m.get(msgId);
  if (!entry) return;
  entry.attempts += 1;
  entry.lastTryAt = Date.now();
  if (error) entry.lastError = error;
}

// getPendingAckStats 用于监控/调试（外部可读取重试队列状态）。
export function getPendingAckStats(channel) {
  const m = _pendingAckByChannel[channel];
  if (!m) return { size: 0 };
  let maxAttempts = 0;
  let oldest = 0;
  const now = Date.now();
  for (const [, entry] of m) {
    if (entry.attempts > maxAttempts) maxAttempts = entry.attempts;
    const age = now - entry.firstSeenAt;
    if (age > oldest) oldest = age;
  }
  return { size: m.size, maxAttempts, oldestAgeMs: oldest };
}

// 初始化：预加载各渠道已发缓存 + 待重试 ack 集合（在轮询开始前调用一次）
export async function initDownlink(channels) {
  for (const ch of channels) {
    await getCache(ch).load();
    this_pendingAckFor(ch); // 确保 _pendingAckByChannel[ch] 已建空 Set
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

  // 2026-08-15 P0-9：先消化上轮残留的 _pendingAck（带退避，attempts<MAX_ACK_RETRY_ATTEMPTS）。
  // 必要性：ack 是"后端记账"，失败不能让"用户已收到"这件事回滚。
  // 节奏：claimDuePendingAck 自动按退避时长筛本轮可重试的条目；逐条重试；attempts++ 直到达上限。
  // 关键：每条 msg_id 单独走 ackOutbox，与正常下发同接口（避免实现分裂）。
  const duePending = claimDuePendingAck(channel);
  if (duePending.length) {
    let successCount = 0;
    let failCount = 0;
    for (const { msgId } of duePending) {
      try {
        const reAck = await ackOutbox(
          { serverUrl, channel, accountId, token },
          [msgId],
          { label: `[下行 ack 重试] ${channel}:${msgId}` }
        );
        // 详细 ack 响应：acked/duplicate 视为成功，not_found 也视为"已处理（无需再发）"
        const handled = processAckDetailedResult(reAck, [msgId], channel, 'reAck');
        const ok = handled.acked + handled.duplicate + handled.not_found;
        if (ok > 0) {
          markPendingAckTried(channel, msgId, true);
          successCount++;
        } else {
          markPendingAckTried(channel, msgId, false,
            reAck && reAck.status === 'ok' ? 'retriable' : 'response_not_ok');
          failCount++;
        }
      } catch (e) {
        markPendingAckTried(channel, msgId, false, String(e && e.message || e));
        failCount++;
      }
    }
    if (successCount || failCount) {
      log.info(`pendingAck 重发 ack 完成`, {
        channel, success: successCount, fail: failCount, retried: duePending.length,
      });
    }
  }

  const res = await getOutbox(
    { serverUrl, channel, accountId, token },
    { label: `[下行 outbox] ${channel}`, batchSize: outboxBatchSize }
  );
  const messages = (res && res.messages) || [];
  // 2026-08-14 用户诉求：下发拉取到的 messages 完整打 console（content 不截断）。
  // pollDownlink 是通道C·下发的入口；http-ingest.getOutbox 已打 request/response，
  // 此处再补一段"分组前全量 messages"——便于排查"服务端拉到了 N 条但本地分组后只下发了 M 条"的对账。
  console.log('[bridge FULL] 下发分组前 messages =', JSON.parse(JSON.stringify(messages)));

  // —— 按 conversation_id 分组（同会话内保留原顺序以维持单会话限速语义）——
  const groups = new Map(); // conversation_id -> [{ msg, sanitized }]
  for (const m of messages) {
    if (!m || !m.msg_id) continue;
    // SentCache key 必须用 msg_id|conversation_id 复合键：
    //   AI 回复 msg_id = contentHash(channel, content)，不含 conversation_id（patrol 回环去重需要）。
    //   跨会话同 content 的 AI 回复 msg_id 相同，若只用 msg_id 做 key，
    //   第一会话 ack 后 cache.add(msg_id) → 第二会话 cache.has(msg_id) 命中 → 跳过，永远不下发！
    //   复合键保证不同会话的同 msg_id 消息各自独立去重。
    // 主动私信场景（后端 Extra.dm_target==='member'）：以 receiver_id（成员标识）为会话定位键，
    // 而非原群会话 conversation_id——抖音私信会话 id 即成员标识，列表匹配可打开已有私信会话。
    let convId = m.conversation_id || '_unknown_';
    if (m.receiver_id && m.receiver_id !== m.conversation_id) {
      let ex = m.extra;
      if (typeof ex === 'string') { try { ex = JSON.parse(ex); } catch (_) { ex = null; } }
      if (ex && ex.dm_target === 'member') convId = m.receiver_id;
    }
    const cacheKey = `${m.msg_id}|${convId}`;
    if (cache.has(cacheKey)) continue; // 已发过，绝不重复
    const raw = m.content || '';
    if (!raw) continue; // 空内容不下发（避免占位空消息打给用户）
    // XSS 防护：净化内容（控制长度、去控制字符）
    const safeContent = sanitizeForDisplay ? sanitizeForDisplay(raw) : raw;
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
            sendOutbound(sanitized, convId, { viaAdapter: channel }),
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
      // 2026-08-14 用户诉求：每条 sendOutbound 结果完整打 console（content 不截断）。
      console.log('[bridge FULL] sendOutbound 结果', {
        channel, conv_id: convId, msg_id: msg.msg_id,
        content: msg.content, sanitized, result,
      });
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
              sendOutbound(sanitized, convId, { viaAdapter: channel }),
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
    // 关键顺序（2026-08-07 修复消息丢失边界 + 2026-08-14 修复重发边界）：
    //   1) 先转发成功 → 2) ack 服务端 → 3) 写本地已发缓存。
    //
    //   2026-08-14 修复（用户诉求 "不要兜底逻辑会导致垃圾数据" 的反例——这里必须"兜底"）：
    //     sendOutbound 成功 = 消息已到达用户手机/聊天框。这一事实是"用户已收到"的唯一权威。
    //     ack 服务端只是后端记账的副作用，绝对不能因为 ack 失败就让前端"装作没发过"——
    //     那会导致：下个轮询周期 server 仍是 pending → 前端又拉到 → 再发给用户 = 重发。
    //     "用户收两条" 比 "后端状态机不一致" 严重得多。
    //
    //     正确策略：
    //       - sendOutbound 成功 → 立刻 cache.add(本地权威防重发)
    //       - 同时尝试 ack 失败 → 记入 _pendingAckIds 队列，本轮仍继续
    //       - 下轮 pollDownlink 进入时先重发 _pendingAckIds 队列（不重发内容，仅重发 ack）
    //       - 重发 N 次后放弃 → 仍保留 cache（用户已收到，后端不一致无影响）
    //
    //   反向兜底禁止：sendOutbound 失败 = 消息没发出 → 不能写 cache（否则永远不下发）。
    if (sentIds.length) {
      // 第 1 步：先写本地已发缓存（不依赖 ack 成功，与"用户是否收到"对齐）
      for (const id of sentIds) cache.add(`${id}|${convId}`);
      // 第 2 步：尝试 ack 服务端（副作用：让 server 知道这条已下发）
      const ackRes = await ackOutbox(
        { serverUrl, channel, accountId, token },
        sentIds,
        { label: `[下行 ack] ${channel}:${convId}` }
      );
      if (ackRes && ackRes.status === 'ok') {
        // 2026-08-15 P3-D + P4-3.4 修复：详细 ack 响应——精确处理每条 msg_id
        //   - status='acked' / 'duplicate' → 已处理完成，不入 _pendingAck
        //   - status='not_found' → 服务端明确"不可能再 ack 成功"，立即停止重发（不写 cache 后续删除由后端）
        //   - items 中无对应 msg_id（响应不完整）→ 入 _pendingAck 下轮重试
        //   - items[].status 缺失或未知 → 入 _pendingAck 下轮重试（保守 retriable）
        //   - 整体响应 ok 但 items 字段缺失（老版本）→ 全量视为成功，不入 pending
        const items = Array.isArray(ackRes.items) ? ackRes.items : null;
        if (items) {
          const itemByMsgID = new Map(items.map((it) => [it.msg_id, it]));
          for (const id of sentIds) {
            const it = itemByMsgID.get(id);
            if (!it) {
              // 响应不完整（无对应 item）→ 保守入 pending 下轮重试
              addPendingAck(channel, id, 'ack_response_missing_item');
              log.warn(`下行 ack 详情缺失 item: 入 _pendingAck 下轮重试`, { channel, conv_id: convId, msg_id: id });
              continue;
            }
            if (it.status === 'acked' || it.status === 'duplicate') {
              // 已处理 → 不入 pending
            } else if (it.status === 'not_found') {
              // 明确停止重发（不重试 ack，但 cache 仍保留以防重发）
              log.warn(`下行 ack 详情: not_found 停止重发`, { channel, conv_id: convId, msg_id: id });
            } else {
              // status 缺失或未知 → 保守入 pending 下轮重试
              addPendingAck(channel, id, `unknown_status_${it.status}`);
              log.warn(`下行 ack 详情未知 status: 入 _pendingAck`, { channel, conv_id: convId, msg_id: id, status: it.status });
            }
          }
        }
        // 老版本响应：全量视为成功，不入 pending（保持兼容）
        allAckIds.push(...sentIds);
      } else {
        // ack 失败但 cache 已写：用户已收到（绝对不重发）。记入 _pendingAckIds 等下轮重发 ack。
        for (const id of sentIds) {
          addPendingAck(channel, id, 'ack_request_failed');
        }
        log.warn(`下行单会话 ack 失败（已发已写 cache 防重发，纳入下轮 ack 重试队列）`, {
          channel, convId, count: sentIds.length,
        });
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

// processAckDetailedResult 处理服务端详细 ack 响应（P3-D 2026-08-15）。
//
// 输入：ackRes（{status, acked, duplicate_count, not_found_count, items: [...]}}）+ ackIds（请求的 msg_ids）
// 输出：{ acked: number, duplicate: number, not_found: number, retriable: number }
//   - acked       本次成功翻转行数（status='acked'）
//   - duplicate   幂等跳过行数（status='duplicate'，本地无需重发）
//   - not_found   不存在行数（status='not_found'，本地无需再尝试）
//   - retriable   需进入下轮 _pendingAck 重试的行数（status 缺失或响应未含详情）
//
// 设计原则：
//   - status='acked' / 'duplicate' 都视为"已处理"，不应再入 pendingAck
//   - status='not_found' 视为"不可能再 ack 成功"，也不应入 pendingAck
//   - 响应无 items 字段（老版本）按"全量成功"处理（保持兼容）
//   - 响应 ok 但部分 msg_id 没有对应 item（极少见）→ 入 pendingAck 下轮重试
export function processAckDetailedResult(ackRes, ackIds, channel = '', label = 'ack') {
  const result = { acked: 0, duplicate: 0, not_found: 0, retriable: 0 };
  if (!ackRes || ackRes.status !== 'ok') {
    // 整体失败：所有 msg_id 都需重试
    result.retriable = Array.isArray(ackIds) ? ackIds.length : 0;
    return result;
  }
  const items = Array.isArray(ackRes.items) ? ackRes.items : null;
  if (!items) {
    // 老版本响应：无 items 详情，按"全量成功"处理
    result.acked = Array.isArray(ackIds) ? ackIds.length : 0;
    return result;
  }
  // 新版本响应：per-msg-id 分类
  const itemByMsgID = new Map(items.map((it) => [it.msg_id, it]));
  for (const id of ackIds || []) {
    if (!id) continue;
    const it = itemByMsgID.get(id);
    if (!it || !it.status) {
      result.retriable++;
      continue;
    }
    if (it.status === 'acked') result.acked++;
    else if (it.status === 'duplicate') result.duplicate++;
    else if (it.status === 'not_found') result.not_found++;
    else result.retriable++; // 未知状态：保守处理为可重试
  }
  return result;
}
