// 服务端 WebSocket 客户端（background 使用）：自动重连 + 心跳 + 帧分发。
// 协议与服务端 user-server/internal/bridge 完全一致（见 frames.go）。
import { FRAME } from './types.js';
import { DEFAULT_USER_SERVER, WS_CLIENT_DEFAULTS } from './constants.js';
import { createLogger } from './logger.js';

const log = createLogger('ws', 'bridge');

function toWsUrl(serverUrl) {
  return serverUrl.replace(/^http/, 'ws');
}

export class BridgeClient {
  constructor(opts) {
    const { serverUrl, channel, accountId, token, conversationId, onOutbound, onOpen, onClose, onError } = opts;
    this.serverUrl = serverUrl;
    this.channel = channel;
    this.accountId = accountId;
    this.token = token;
    this.conversationId = conversationId;
    this.WS = opts.WebSocket || (typeof WebSocket !== 'undefined' ? WebSocket : null);
    this.onOutbound = onOutbound || (() => {});
    this.onOpen = onOpen || (() => {});
    this.onClose = onClose || (() => {});
    this.onError = onError || (() => {});
    this.ws = null;
    this.alive = false;
    this._pingTimer = null;
    this._reconnectTimer = null;
  }

  _buildUrl() {
    // WS 端点路径来自 DEFAULT_USER_SERVER.wsPath（与 user-server/internal/router/service_routes.go:90 一致）
    const u = new URL(`${toWsUrl(this.serverUrl)}${DEFAULT_USER_SERVER.wsPath}`);
    u.searchParams.set('channel', this.channel);
    u.searchParams.set('account_id', this.accountId || '');
    if (this.conversationId) u.searchParams.set('conversation_id', this.conversationId);
    if (this.token) u.searchParams.set('token', this.token);
    return u.toString();
  }

  connect() {
    if (this.ws && (this.ws.readyState === 1 || this.ws.readyState === 0)) return;
    let ws;
    try {
      ws = new this.WS(this._buildUrl());
    } catch (e) {
      log.error('WS 构造失败', e);
      this._scheduleReconnect();
      return;
    }
    this.ws = ws;
    ws.onopen = () => {
      this.alive = true;
      this._reconnectAttempts = 0;
      log.info('已连接服务端', this.channel, this.accountId);
      // 首帧注册
      this.send({ type: FRAME.REGISTER, channel: this.channel, account_id: this.accountId, conversation_id: this.conversationId || '' });
      this.onOpen();
      this._startPing();
    };
    ws.onmessage = (ev) => this._onMessage(ev);
    ws.onclose = () => {
      this.alive = false;
      this._stopPing();
      this.onClose();
      this._scheduleReconnect();
    };
    ws.onerror = (e) => {
      log.error('WS 错误', e);
      this.onError(e);
    };
  }

  _onMessage(ev) {
    let frame;
    try {
      frame = JSON.parse(ev.data);
    } catch (e) {
      log.error('非法帧', ev.data);
      return;
    }
    switch (frame.type) {
      case FRAME.OUTBOUND:
        if (frame.reply) this.onOutbound(frame.reply);
        break;
      case FRAME.PONG:
        this.alive = true;
        break;
      case FRAME.ERROR:
        log.error('服务端错误:', frame.message);
        break;
      default:
        break;
    }
  }

  // I2 修复：服务端 gorilla/websocket 用 SetPongHandler 处理协议级 pong 帧（非 JSON ping）。
  // 浏览器 WebSocket 无法主动发协议级 ping；改为发送 JSON {type:"pong"} 帧，
  // 服务端 handleFrame 在 FramePong 分支会调用 SetReadDeadline 重置 deadline（见 handler.go）。
  // 同时配合 onclose 重置 alive 标志；服务端 60s 内必收到任一帧（ping 帧或 inbound 帧）。
  // 周期由 WS_CLIENT_DEFAULTS.serverIdleTimeoutMs 单源管理（见 constants.js / DEFAULTS.md）
  _startPing() {
    this._stopPing();
    this._pingTimer = setInterval(() => {
      if (this.alive) {
        // alive 标志由任何下行帧（pong/error/outbound）重置；若长期无下行，发送 JSON pong 维持
        this.send({ type: FRAME.PONG });
        this.alive = false; // 等待服务端下次下行才重新置真；若 serverIdleTimeoutMs 内无任何帧，触发 onclose
      } else {
        log.warn('服务端 serverIdleTimeoutMs 无任何帧响应，主动断开重连');
        try { this.ws.close(); } catch (_) { /* noop */ }
      }
    }, WS_CLIENT_DEFAULTS.serverIdleTimeoutMs);
  }

  _stopPing() {
    if (this._pingTimer) clearInterval(this._pingTimer);
    this._pingTimer = null;
  }

  // I3 修复：指数退避重连（1s -> 2s -> 4s -> 8s -> 16s，封顶 30s）
  // 参数来源：WS_CLIENT_DEFAULTS（见 constants.js 文档源）
  _scheduleReconnect() {
    if (this._reconnectTimer) return;
    this._reconnectAttempts = (this._reconnectAttempts || 0) + 1;
    const { reconnectBaseMs, reconnectMaxMs, reconnectJitterMs } = WS_CLIENT_DEFAULTS;
    const base = Math.min(reconnectMaxMs, reconnectBaseMs * Math.pow(2, this._reconnectAttempts - 1));
    const delay = Math.min(reconnectMaxMs, base + Math.floor(Math.random() * reconnectJitterMs));
    log.info(`将在 ${delay}ms 后重连（第 ${this._reconnectAttempts} 次）`);
    this._reconnectTimer = setTimeout(() => {
      this._reconnectTimer = null;
      this.connect();
    }, delay);
  }

  send(frame) {
    if (this.ws && this.ws.readyState === 1) {
      this.ws.send(JSON.stringify(frame));
      return true;
    }
    return false;
  }

  sendInbound(message) {
    return this.send({ type: FRAME.INBOUND, message });
  }

  sendHistory(message) {
    return this.send({ type: FRAME.HISTORY, message });
  }

  close() {
    this._stopPing();
    if (this._reconnectTimer) clearTimeout(this._reconnectTimer);
    if (this.ws) this.ws.close();
    this.ws = null;
    this.alive = false;
  }
}
