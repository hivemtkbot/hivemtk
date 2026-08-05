// Bridge HTTP Transport (2026-08-05 架构重构)
//
// 用户诉求：bridge 模块不再维护 WebSocket 长连接，改用 HTTP 长轮询上报到
//   POST /api/bridge/ingest。所有渠道（douyin/xiaohongshu/tiktok/xianyu/kuaishou）
//   共用同一 HTTP client + 同一 URL 构造 + 同一日志打印复用层。
//
// 设计要点：
//   1. 0 状态：每次调用 buildIngestUrl / logRequest 都是纯函数；
//      无 reconnect 状态机、无 zombie 检测、无 SW 冻结适配
//   2. fetch 自动重试：浏览器 fetch 失败由 transport 层捕获并按 1s/2s/4s 退避重试
//   3. 集中打印：所有渠道的"上行 URL + 全部 query + body"经由 _logRequest
//      统一格式打印，与 user-server 侧 [Bridge HTTP] 收到 ingest 请求 日志对照
//   4. 500s 长轮询：长轮询时服务端处理 AI 推理后直接返回 reply，
//      transport 端点拿到后 dispatch 到对应 content script 触发 sendOutbound
//
// 替换关系：原 bridge-client.js (WS) + registry.js 被本文件 + polling-loop.js 替代
//   - buildIngestUrl：等价于原 _buildUpstreamUrl
//   - describeIngestParams：等价于原 describeUpstreamParams
//   - logRequest：等价于原 _logUpstream
import { DEFAULT_USER_SERVER } from './constants.js';
import { createLogger } from './logger.js';

const log = createLogger('http', 'bridge');

// HTTP ingest 端点路径（与 user-server/internal/bridge/handler_http.go POST /api/bridge/ingest 严格对齐）
const INGEST_PATH = '/api/bridge/ingest';

// 长轮询/连接默认参数
const HTTP_INGEST_DEFAULTS = Object.freeze({
  // 长轮询默认等待时间：500 秒（与 user-server HTTPPollingMaxTimeout 对齐）
  longPollTimeoutMs: 500000,
  // 长轮询时长（每次请求实际等待的毫秒，扩展侧主动控制更短，便于定期回写心跳）
  requestTimeoutMs: 30000,
  // fetch 重试：最多 3 次，退避 1s/2s/4s
  maxRetries: 3,
  retryBaseMs: 1000,
  // 单条消息最大体积：4KB（与 user-server maxReplyContentBytes 对齐）
  maxContentBytes: 4 * 1024,
});

// 把 serverUrl 的 scheme 切换为 http(s)（ingest 是 HTTP，非 WS）
function toHttpUrl(serverUrl) {
  return serverUrl.replace(/^ws/, 'http');
}

// 构造完整的上行 URL（HTTP ingest 端点 + channel/account_id/conversation_id/token 全部 query 参数）。
//
// 文档源：DEFAULT_USER_SERVER.baseUrl（用户配置或默认）与 user-server internal/router/router.go:337
// （POST /api/bridge/ingest）严格对齐。
//
// 入参：
//   - serverUrl: 用户配置的 baseUrl（http/https/ws/wss 都接受，自动归一为 http(s)）
//   - params: { channel, accountId, conversationId, token }
// 返回：完整 URL 字符串，可直接传给 fetch。
function buildIngestUrl(serverUrl, params) {
  const u = new URL(`${toHttpUrl(serverUrl)}${INGEST_PATH}`);
  u.searchParams.set('channel', params.channel || '');
  u.searchParams.set('account_id', params.accountId || '');
  if (params.conversationId) u.searchParams.set('conversation_id', params.conversationId);
  if (params.token) u.searchParams.set('token', params.token);
  return u.toString();
}

// 把 URL 解析为可读参数表（仅用于日志展示，不影响真实请求）。
// 真实请求仍由 URL + searchParams 决定，此处仅做 mirror 打印。
function describeIngestParams(url) {
  try {
    const u = new URL(url);
    const out = {};
    for (const [k, v] of u.searchParams.entries()) {
      // token 不在日志里明文输出（私域部署仍按隐私基线脱敏）
      if (k === 'token') {
        out[k] = v ? `${v.slice(0, 4)}***(${v.length} chars)` : '';
      } else {
        out[k] = v;
      }
    }
    return {
      origin: u.origin,
      path: u.pathname,
      query: out,
    };
  } catch (_) {
    return { url, parseError: true };
  }
}

// 截断 body 用于日志预览（与 user-server 端 readBodyPreview 4KB 上限对齐）
function previewBody(body, maxBytes = 4096) {
  if (body == null) return { preview: '', size: 0, truncated: false };
  const s = typeof body === 'string' ? body : JSON.stringify(body);
  if (s.length <= maxBytes) {
    return { preview: s, size: s.length, truncated: false };
  }
  return { preview: s.slice(0, maxBytes) + `... [truncated, total=${s.length} bytes]`, size: s.length, truncated: true };
}

// _logRequest 统一打印 HTTP ingest 上行（完整 URL + 解析后的 origin/path/query 字段 + body 预览）。
// 所有渠道（douyin/xiaohongshu/tiktok/xianyu/kuaishou）共用，格式与
// user-server 侧 collectHTTPRequestInfo 输出严格对齐，便于日志对照定位。
//
// 失败（URL 构造抛错）返回 null，调用方需自行处理。
// 成功返回 { url, payload }：url 给 fetch 使用；payload 给测试 / 调试使用。
function _logRequest(label, serverUrl, params, body, extra) {
  let url = '';
  let parsed = null;
  try {
    url = buildIngestUrl(serverUrl, params);
    parsed = describeIngestParams(url);
  } catch (e) {
    log.error(`${label} URL 构造失败`, e);
    return null;
  }
  const bodyInfo = previewBody(body);
  // 提取消息核心内容：用户最关心"上行了什么消息"，但 body_preview 被截断后看不到
  const msgs = Array.isArray(body && body.messages) ? body.messages : [];
  const messagesSummary = msgs.map((m) => ({
    sender: m.sender_type || '?',
    content: (m.content || '').slice(0, 80),
    event_id: (m.event_id || '').slice(-12),
  }));
  const payload = {
    url,
    ...(parsed || {}),
    body_preview: bodyInfo.preview,
    body_size: bodyInfo.size,
    body_truncated: bodyInfo.truncated,
    messages_count: msgs.length,
    messages_summary: messagesSummary,
    ...(extra || {}),
  };
  // 关键：URL/对象作为 console 第二参数传入，避免 logger.sanitizeArgs 对第一个字符串
  // 参数按 24 字符截断（"addr=http://..."）。对象内的 url 字段是完整 URL，
  // 用户可从 console 展开对象查看全部参数。
  log.info(label, payload);
  return { url, payload };
}

// fetchWithRetry 带指数退避的 fetch。
// 失败重试：1s → 2s → 4s（封顶 8s），最多 maxRetries 次。
// 超时由外部 AbortController 控制（每个 fetch 用同一个 controller）。
//
// 重试语义（HTTP 标准）：
//   - 4xx（除 408）客户端错误：直接抛错，不重试（重试无意义）
//   - 408 Request Timeout：可重试（服务端忙，下次可能 OK）
//   - 5xx 服务端错误：可重试
//   - 网络错误（fetch reject）：可重试
async function fetchWithRetry(url, options, retryOpts = {}) {
  const maxRetries = retryOpts.maxRetries ?? HTTP_INGEST_DEFAULTS.maxRetries;
  const retryBaseMs = retryOpts.retryBaseMs ?? HTTP_INGEST_DEFAULTS.retryBaseMs;
  let lastErr = null;
  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      const res = await fetch(url, options);
      if (!res.ok) {
        const text = await res.text().catch(() => '');
        // 4xx（除 408）标记为不可重试：客户端错误，重试无意义
        const is4xxNonRetryable =
          res.status >= 400 && res.status < 500 && res.status !== 408;
        const err = new Error(
          `HTTP ${res.status} ${res.statusText}: ${text.slice(0, 200)}`
        );
        if (is4xxNonRetryable) err.nonRetryable = true;
        throw err;
      }
      return res;
    } catch (e) {
      lastErr = e;
      // 4xx 客户端错误：直接跳出重试循环
      if (e && e.nonRetryable) break;
      if (attempt >= maxRetries) break;
      // 指数退避 + 抖动
      const delay = Math.min(8000, retryBaseMs * Math.pow(2, attempt)) + Math.floor(Math.random() * 200);
      log.warn(`fetch 失败，${delay}ms 后重试 #${attempt + 1}/${maxRetries}`, { error: String(e && e.message || e) });
      await new Promise((r) => setTimeout(r, delay));
    }
  }
  throw lastErr || new Error('fetch failed');
}

// postIngest 上报一条 ingest 请求（带统一日志 + 退避重试）。
//
// 入参：
//   - serverUrl, channel, accountId, conversationId, token：URL 参数
//   - body: 已构造的请求体（HTTPIngestRequest，与 user-server 端严格对齐）
//   - opts: { timeoutMs, expectReply, label }
// 返回：HTTPIngestResponse
//
// 行为：
//   1) 先调 _logRequest 打印 URL + body 预览
//   2) 调 fetchWithRetry 实际请求（带超时）
//   3) 解析响应后打印响应日志
//   4) 错误向上抛，由调用方决定重试/降级
async function postIngest({ serverUrl, channel, accountId, conversationId, token }, body, opts = {}) {
  const params = { channel, accountId, conversationId, token };
  const label = opts.label || '[HTTP ingest]';
  const logResult = _logRequest(label, serverUrl, params, body, { expect_reply: !!body.expect_reply });
  if (!logResult) {
    throw new Error('ingest URL 构造失败');
  }
  const { url } = logResult;
  const timeoutMs = opts.timeoutMs ?? HTTP_INGEST_DEFAULTS.requestTimeoutMs;
  // 透传 retry 参数：测试场景可显式覆盖 maxRetries/retryBaseMs；
  // 生产默认走 HTTP_INGEST_DEFAULTS（maxRetries=3, retryBaseMs=1000）。
  const retryOpts = {
    maxRetries: opts.maxRetries ?? HTTP_INGEST_DEFAULTS.maxRetries,
    retryBaseMs: opts.retryBaseMs ?? HTTP_INGEST_DEFAULTS.retryBaseMs,
  };
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  const startedAt = Date.now();
  let responsePayload = null;
  try {
    // 只在 traceId 非空时才加 X-Trace-Id 头：空字符串仍算自定义头，
    // 会触发 CORS preflight 且要求服务端 ACAH 白名单包含该头。
    const headers = { 'Content-Type': 'application/json' };
    if (opts.traceId) headers['X-Trace-Id'] = opts.traceId;
    const res = await fetchWithRetry(url, {
      method: 'POST',
      headers,
      body: JSON.stringify(body),
      signal: controller.signal,
    }, retryOpts);
    const text = await res.text();
    let json = null;
    try {
      json = text ? JSON.parse(text) : null;
    } catch (e) {
      log.error('ingest 响应 JSON 解析失败', e, { status: res.status, text: text.slice(0, 200) });
      throw new Error('ingest 响应非 JSON: ' + text.slice(0, 100));
    }
    responsePayload = json;
    return json;
  } catch (e) {
    log.error('ingest POST 失败', e, { url, elapsed_ms: Date.now() - startedAt });
    throw e;
  } finally {
    clearTimeout(timer);
    // 响应日志（成功/失败均打）
    if (responsePayload) {
      const out = responsePayload.outbound_replies || [];
      log.info('ingest 响应', {
        url,
        ok: responsePayload.ok,
        elapsed_ms: Date.now() - startedAt,
        ingested_count: (responsePayload.ingested || []).length,
        outbound_replies_count: out.length,
        outbound_replies_preview: out.map((r) => ({
          channel: r && r.channel,
          conversation_id: r && r.conversation_id,
          content_preview: r && r.content ? r.content.slice(0, 100) : '',
          content_length: r && r.content ? r.content.length : 0,
          reply_to_event_id: r && r.reply_to_event_id,
        })),
        reason: responsePayload.reason,
      });
    }
  }
}

export {
  INGEST_PATH,
  HTTP_INGEST_DEFAULTS,
  buildIngestUrl,
  describeIngestParams,
  previewBody,
  _logRequest,
  fetchWithRetry,
  postIngest,
};
