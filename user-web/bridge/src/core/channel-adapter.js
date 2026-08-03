// 渠道适配器基类：统一「监听私信 / 去重 / 会话切换回填空历史 / 上行上报 / 下行回写 + 限速风控」
//
// 数据流（需求③ ⑤）：
//   DOM 监听 ── 客户消息 ──▶ inbound_message 帧 ──▶ 服务端（触发 AI）
//   DOM 监听 ── 自己/历史消息 ──▶ history 帧（direction=outbound）──▶ 服务端（仅落库）
//   页面加载/会话切换 存量消息 ──▶ history 帧（inbound/outbound）──▶ 服务端（仅落库，不触发 AI）
//   服务端 AI 回复 ──▶ outbound_reply 帧 ──▶ 本适配器 sendOutbound ──▶ 回写网页 ──▶ history 帧(direction=outbound)
//
// 防回环三层（需求④）：① 自/他判定只上行客户消息给 AI；② 自己气泡仅落库不进 AI；③ 限速器冷却+去重。
import { createLogger } from './logger.js';
import { RateLimiter } from './rate-limiter.js';
import { makeUnifiedMessage, SENDER, DIRECTION } from './types.js';

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

export class BaseAdapter {
  constructor({ name, channel, SEL, hooks, rateLimiter }) {
    this.name = name;
    this.channel = channel;
    this.SEL = SEL || {};
    this.hooks = hooks || {};
    this.rateLimiter = rateLimiter || new RateLimiter();
    this.account = null;
    this.conversationId = null;
    this.seen = new Set();          // 已处理消息的内容指纹（防 DOM 重复 + 防回环）
    this.observer = null;
    this.convPollTimer = null;
    this.fallbackTimer = null;
    this.activeRoot = null;
    this._graceTimer = null;
    // 历史宽限期（ms）：会话初次挂载/切换后的一段时间内，新出现的客户消息仅回填历史（落库），
    // 不触发 AI 自动回复。避免打开含存量私信的会话时被当成新消息逐一自动回复
    // （用户诉求：只同步历史、不发消息）。钩子可覆盖 this.historyGraceMs。
    this.historyGraceMs = (hooks && hooks.historyGraceMs) || 8000;
    this.historyGraceUntil = 0;
    // 修复：用 Map 替代数组存储 recentSelf 指纹，便于 O(1) 查询；
    // 上限 200 条防止异常场景下无限增长，超过时清理最旧一半。
    this.recentSelf = new Map();
    this.recentSelfMax = 200;
    this.callbacks = {};
    this.log = createLogger(channel, 'adapter');
  }

  setAccount(account) { this.account = account; }

  // ---- hooks 透传（保持三渠道现有签名不变）----
  match() { return this.hooks.match ? this.hooks.match() : false; }
  // matchMode: 区分严格匹配 vs fallback 降级匹配。
  //   - 'strict'  严格选择器命中（最理想）
  //   - 'fallback' 平台严格选择器失效、走通用 DOM 扫描（提示维护者更新 SEL）
  //   - null       match() 返回 false（页面不匹配）
  matchMode() {
    if (!this.match()) return null;
    if (this.hooks.matchMode) return this.hooks.matchMode();
    return 'strict';
  }
  snapshotMeta() { return this.hooks.snapshotMeta ? this.hooks.snapshotMeta() : {}; }
  getAccountId() { return this.hooks.getAccountId ? this.hooks.getAccountId() : (this.account || ''); }
  getConversationId() { return this.hooks.getConversationId ? this.hooks.getConversationId() : (this.snapshotMeta().conversationId || null); }
  // 消息线程根：优先 hooks.getMessageRoot；未实现则回退到 getMessageListRoot。
  // 关键修复：抖音等渠道只实现了 getMessageListRoot，缺失 getMessageRoot 会导致
  // _attachConversation 拿到 null → 历史回填(_backfill)与 MutationObserver 永不生效，
  // 存量私信只能被 3s 兜底扫描当实时 INBOUND 触发 AI（误发消息）。
  getMessageRoot() {
    if (this.hooks.getMessageRoot) {
      const r = this.hooks.getMessageRoot();
      if (r) return r;
    }
    return this.hooks.getMessageListRoot ? this.hooks.getMessageListRoot() : null;
  }
  getMessageItems() { return this.hooks.getMessageItems ? this.hooks.getMessageItems() : []; }
  parseMessageItem(item) { return this.hooks.parseMessageItem ? this.hooks.parseMessageItem(item) : null; }
  rawSendText(text) { return this.hooks.sendText ? this.hooks.sendText(text) : Promise.resolve(false); }
  selfTest() { return this.hooks.selfTest ? this.hooks.selfTest() : []; }

  start(callbacks = {}) {
    this.callbacks = callbacks;
    if (!this.match()) {
      this.log.warn('页面不匹配，适配器未启动');
      return false;
    }
    const meta = this.snapshotMeta();
    this.account = meta.accountId || this.account;
    this.conversationId = this.getConversationId();
    this._attachConversation();
    this._startConvPolling();
    return true;
  }

  stop() {
    if (this.observer) this.observer.disconnect();
    if (this.convPollTimer) clearInterval(this.convPollTimer);
    if (this.fallbackTimer) clearInterval(this.fallbackTimer);
    if (this._graceTimer) clearTimeout(this._graceTimer);
  }

  // 会话切换探测：切换后重挂载观察器并回填新会话历史
  _startConvPolling() {
    this.convPollTimer = setInterval(() => {
      const cid = this.getConversationId();
      if (cid && cid !== this.conversationId) {
        this.log.info(`会话切换: ${this.conversationId} -> ${cid}`);
        this.conversationId = cid;
        this._attachConversation();
      }
    }, 2000);
  }

  _attachConversation() {
    const root = this.getMessageRoot();
    this.activeRoot = root;
    if (root) {
      this._observe(root);
      this._backfill(root); // 初次/切换时立即回填（线程已渲染则生效）
    }
    // 历史宽限期：挂载/切换后的窗口内，新出现的客户消息仅回填历史、不触发 AI
    // （存量私信同步到系统，但不自动回复）。
    this.historyGraceUntil = Date.now() + this.historyGraceMs;
    // 延迟回填：私信线程常异步渲染，1.5s 后再填一次，覆盖初次 _backfill 时尚未渲染的气泡。
    if (this._graceTimer) clearTimeout(this._graceTimer);
    this._graceTimer = setTimeout(() => {
      const r = this.getMessageRoot();
      if (r) this._backfill(r);
    }, 1500);
    // 兜底轮询（MutationObserver 漏抓时的保险）
    if (this.fallbackTimer) clearInterval(this.fallbackTimer);
    this.fallbackTimer = setInterval(() => this._scanIncremental(), 3000);
  }

  _observe(root) {
    if (this.observer) this.observer.disconnect();
    this.observer = new MutationObserver((muts) => this._onMutations(muts));
    this.observer.observe(root, { childList: true, subtree: true, characterData: true });
  }

  _addedElements(muts) {
    const out = [];
    for (const m of muts) {
      if (m.addedNodes) {
        for (const n of m.addedNodes) {
          if (n.nodeType === 1) out.push(n);
        }
      }
    }
    return out;
  }

  _onMutations(muts) {
    const nodes = this._addedElements(muts);
    for (const node of nodes) {
      const items = node.matches && node.matches(this.SEL.MSG_ITEM)
        ? [node]
        : (node.querySelectorAll ? Array.from(node.querySelectorAll(this.SEL.MSG_ITEM)) : []);
      for (const item of items) this._handleIncremental(item);
    }
  }

  // 兜底全量扫描（与去重结合，安全可重复调用）
  _scanIncremental() {
    const items = this.getMessageItems();
    for (const item of items) this._handleIncremental(item);
  }

  _keyOf(parsed) {
    const cid = this.getConversationId() || this.conversationId || '_';
    const ts = Math.floor((parsed.timestamp || Date.now()) / 1000);
    return `${cid}:${parsed.sender_type}:${(parsed.text || '').slice(0, 200)}:${ts}`;
  }

  _handleIncremental(item) {
    const parsed = this.parseMessageItem(item);
    if (!parsed || !parsed.text) return;
    const key = this._keyOf(parsed);
    if (this.seen.has(key)) return;
    this.seen.add(key);
    if (parsed.sender_type === SENDER.CUSTOMER) {
      // 历史宽限期内（首次挂载/会话切换后）：客户消息仅回填历史、不触发 AI，
      // 避免对存量私信逐一自动回复（用户诉求：只同步历史、不发消息）。宽限期后视为实时新消息。
      if (Date.now() < this.historyGraceUntil) {
        this._emitHistory(parsed, DIRECTION.INBOUND);
      } else {
        this._emitInbound(parsed);
      }
    } else {
      // 自己/AI 气泡：仅落库（若是我们刚回写的，跳过避免重复）
      if (this._isRecentSelf(key)) return;
      this._emitHistory(parsed, DIRECTION.OUTBOUND);
    }
  }

  // 回填存量消息（页面加载 / 会话切换）：一律仅落库，不触发 AI
  _backfill(root) {
    const items = this.getMessageItems();
    let n = 0;
    for (const item of items) {
      const parsed = this.parseMessageItem(item);
      if (!parsed || !parsed.text) continue;
      const key = this._keyOf(parsed);
      if (this.seen.has(key)) continue;
      this.seen.add(key);
      const direction = parsed.sender_type === SENDER.CUSTOMER ? DIRECTION.INBOUND : DIRECTION.OUTBOUND;
      this._emitHistory(parsed, direction);
      n++;
    }
    this.log.info(`回填会话 ${this.conversationId}: ${items.length} 条消息`);
  }

  _emitInbound(parsed) {
    const msg = makeUnifiedMessage({
      channel: this.channel,
      account_id: this.getAccountId(),
      conversation_id: this.getConversationId(),
      sender_type: parsed.sender_type,
      content: parsed.text,
      media_url: parsed.media_url || '',
      timestamp: parsed.timestamp,
      event_id: parsed.message_id,
      raw: parsed.raw || null,
    });
    this.log.info('上行实时私信:', (msg.content || '').slice(0, 40));
    if (this.callbacks.onInbound) this.callbacks.onInbound(msg);
  }

  _emitHistory(parsed, direction) {
    const msg = makeUnifiedMessage({
      channel: this.channel,
      account_id: this.getAccountId(),
      conversation_id: this.getConversationId(),
      sender_type: parsed.sender_type,
      content: parsed.text,
      media_url: parsed.media_url || '',
      timestamp: parsed.timestamp,
      direction,
      event_id: parsed.message_id,
      raw: parsed.raw || null,
    });
    this.log.info(`回填空历史(${direction}):`, (msg.content || '').slice(0, 40));
    if (this.callbacks.onHistory) this.callbacks.onHistory(msg, direction);
  }

  _isRecentSelf(key) {
    const ts = this.recentSelf.get(key);
    if (ts === undefined) return false;
    if (Date.now() - ts > 5000) {
      this.recentSelf.delete(key);
      return false;
    }
    return true;
  }

  _markRecentSelf(key) {
    // 修复：超过上限时清理最旧一半，防止异常场景下无限增长
    if (this.recentSelf.size >= this.recentSelfMax) {
      const entries = Array.from(this.recentSelf.entries()).sort((a, b) => a[1] - b[1]);
      const half = Math.floor(entries.length / 2);
      for (let i = 0; i < half; i++) this.recentSelf.delete(entries[i][0]);
    }
    this.recentSelf.set(key, Date.now());
  }

  // 下行回写（带限速风控）：AI 回复经此回写到网页，并上报 outbound 历史
  // 返回 true 表示已回写；false 表示被风控拦截或失败。
  async sendOutbound(text) {
    if (!text) { this.log.warn('回复内容为空，跳过'); return false; }
    const account = this.getAccountId();
    const conv = this.getConversationId();
    const decision = this.rateLimiter.tryAcquire(this.channel, account, conv, text);
    if (!decision.allowed) {
      this.log.warn(`下行被风控拦截: ${decision.reason}`);
      if (this.callbacks.onRateLimited) this.callbacks.onRateLimited(decision);
      return false;
    }
    if (decision.waitHintMs > 0) await sleep(decision.waitHintMs);
    let ok = false;
    try {
      await this.rawSendText(text); // 无异常即视为回写成功（各渠道 sendText 成功时返回 undefined）
      ok = true;
    } catch (e) {
      this.log.error('回写失败', e);
      return false;
    }
    if (ok) {
      this.rateLimiter.markSent(this.channel, account, conv, text);
      const key = this._keyOf({ sender_type: SENDER.AGENT, text, timestamp: Date.now() });
      this._markRecentSelf(key);
      // 上报 outbound 历史（仅落库；Mutation 抓到的同源气泡会被 _isRecentSelf 去重）
      this._emitHistory(
        { sender_type: SENDER.AGENT, text, media_url: '', timestamp: Date.now(), message_id: `out-${Date.now()}` },
        DIRECTION.OUTBOUND
      );
    }
    return ok;
  }
}
