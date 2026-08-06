// 渠道适配器基类：统一「监听私信 / 会话切换回填 / 上行上报 / 下行回写 + 限速风控」
//
// 2026-08-05 架构重构（纯桥接）：
//   - Bridge 只做桥接：DOM 扫描 → 解析 → 上报，不做业务判断（是否入库 / 是否回复由后端判断）
//   - 所有消息（customer / self / agent）统一走 _emitMessage → onMessage 回调上报
//   - 保留 sender_type 标记（消息属性，供后端判断）
//   - 保留 seenNodes WeakSet（防 DOM 重复扫描的技术手段，非业务去重）
//   - 移除 seen Set（内容指纹去重，交给后端）
//   - 移除 recentSelf Map（防回环判断，后端通过 sender_type 判断）
//   - 移除 _suppressBackfill / 历史宽限期（不做业务判断）
//
// 数据流：
//   DOM 监听 ── 任意消息 ──▶ onMessage 帧 ──▶ 服务端（统一收信中心判断是否入库/回复）
//   页面加载/会话切换 存量消息 ──▶ onMessage 帧（history[] 含多轮）──▶ 服务端
//   服务端 AI 回复 ──▶ outbound_replies ──▶ sendOutbound ──▶ 回写网页
import { createLogger } from './logger.js';
import { RateLimiter } from './rate-limiter.js';
import { makeUnifiedMessage, SENDER, DIRECTION, HISTORY_CONTEXT_WINDOW, PATROL_DEFAULTS } from './types.js';
import { mergeSelectors } from './selector-ai.js';
import { simulateRealClick } from './dom.js';

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
    // seenNodes：防 DOM 重复扫描的技术手段（按节点身份去重，避免列表项/同名消息被反复扫描重复上行）。
    // 这是 DOM 扫描必需的，不是业务去重。内容指纹去重交给后端统一收信中心。
    this.seenNodes = new WeakSet();
    this.observer = null;
    this.convPollTimer = null;
    this.fallbackTimer = null;
    this.activeRoot = null;
    this._graceTimer = null;
    this.callbacks = {};
    this.log = createLogger(channel, 'adapter');
    // 会话实时上下文窗口：per-conversation 最近 N 轮（供 inbound 帧携带多轮历史，点3 需求）
    this._convWindow = new Map();
    // 当前页面域名（start 时填充，供持久化键区分账号）
    this.domain = '';
    // —— 私信会话全量遍历同步（一个私信=一个会话=统一收信中心一条记录；遍历所有私信上报）——
    this._syncedConvIds = new Set();  // 已同步会话 id（持久化 localStorage，刷新/重连续传）
    this._syncingAll = false;         // 防止并发重复触发
    this._initialSyncDone = false;    // 首次自动同步只跑一次
    this._rescanTimer = null;         // 周期重扫（捕获新增私信）     // 上次刷新云端选择器的时间戳（冷却用）
    // —— 巡检制度（patrol）：一轮巡检完成自动进入下一轮，遍历左侧会话列表，
    //    对有新消息（未读）的会话点击进入右侧聊天页，捕获新消息上行（触发 AI 自动对话）。
    //    已 seen 的消息靠去重跳过，故只有真正新增的消息会上行 —— 实现自动对话 + 不重复打扰。
    this._patrolTimer = null;
    this._patrolling = false;
    this._patrolOpts = null;          // 当前巡检配置 { intervalMs, throttleMs, ... }
    this._patrolStats = { rounds: 0, visited: 0, withNew: 0, captured: 0, failures: 0, lastRoundAt: 0, lastDurationMs: 0 };
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
  // 优先用渠道自定义 snapshotMeta；否则回落到标准 getter（getAccountId/getConversationId）。
  // 关键修复：抖音等渠道未实现 hooks.snapshotMeta，旧版返回 {} 导致 accountId 永远
  // undefined → common.js 发的 REGISTER 帧 account_id 为空 → 服务端 WS 401 → 数据全不上行。
  // 这也是「自检面板显示 account=douyin_web-unknown，但握手 URL 仍 account_id=空」的根因。
  snapshotMeta() {
    if (this.hooks.snapshotMeta) return this.hooks.snapshotMeta();
    return {
      accountId: this.getAccountId() || '',
      conversationId: this.getConversationId() || null,
    };
  }
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
  // 会话列表枚举：渠道钩子返回 [{ id, name, el }]，遍历器据此逐个打开并回填历史。
  // 未实现则回退空（不遍历）。
  getConversationList() {
    return this.hooks.getConversationList ? this.hooks.getConversationList() : [];
  }
  rawSendText(text) { return this.hooks.sendText ? this.hooks.sendText(text) : Promise.resolve(false); }
  selfTest() { return this.hooks.selfTest ? this.hooks.selfTest() : []; }

  // ============================================================
  // 2026-08-05 HTTP-only 重构：新增 getMessages 公开方法
  //
  // 背景：Bridge 改用 HTTP 长轮询后，content 端不再走 WebSocket 上行，
  // 改由 PollingLoop 每秒巡检：调 getMessages 一次性抓当前会话全部可见消息，
  // 调 postIngest 上报到 /api/bridge/ingest。
  //
  // 行为：
  //   - 调 getMessageItems() 拿 DOM 节点列表
  //   - 逐个调 parseMessageItem() 解析为结构化对象
  //   - 过滤非 text / 空内容（与 _ingest 一致）
  //   - 统一字段命名（PollingLoop 直接消费）：{ message_id, sender_id, sender_name,
  //     sender_type, text, msg_type, timestamp, is_group, group_id, group_name }
  //   - 不修改 seenNodes（PollingLoop 自己做去重）
  //   - 限制 limit（默认 100）防止 OOM
  // ============================================================
  getMessages({ limit = 100 } = {}) {
    const out = [];
    let items = [];
    try { items = this.getMessageItems() || []; } catch (_) { items = []; }
    for (const item of items) {
      if (out.length >= limit) break;
      if (!item) continue;
      let parsed;
      try { parsed = this.parseMessageItem(item); } catch (_) { continue; }
      if (!parsed) continue;
      // 与 _ingest 一致：只取文字消息
      if (parsed.msg_type && parsed.msg_type !== 'text') continue;
      if (!parsed.text) continue;
      out.push({
        message_id: parsed.message_id || '',
        sender_id: parsed.sender_id || '',
        sender_name: parsed.sender_name || '',
        sender_type: parsed.sender_type || SENDER.CUSTOMER,
        text: parsed.text || '',
        msg_type: parsed.msg_type || 'text',
        timestamp: parsed.timestamp || Date.now(),
        is_group: !!parsed.is_group,
        group_id: parsed.group_id || '',
        group_name: parsed.group_name || '',
        media_url: parsed.media_url || '',
      });
    }
    return out;
  }

  start(callbacks = {}) {
    // 顶层 panic guard（2026-08-05 审计 P0 修复）：
    //   原 start() 无 try/catch，任何 adapter 钩子（match/snapshotMeta/_attachConversation 等）
    //   抛错都会冒泡到 content script 顶层 → 整页桥接功能失效，其他渠道也连带挂掉。
    //   单 adapter 异常应仅影响自身：catch 后清理已分配资源（this.stop()），返回 false，
    //   其他渠道继续运行。这是与 BaseAdapter 「单适配器故障不阻断其他渠道」原则一致。
    try {
      // 先停止旧观察者/定时器/巡检，防止 sync() 重激活时双重 emit
      // （旧 MutationObserver + 新 MutationObserver → FrameInbound ×2 → AI重复回复）
      this.stop();
      this.callbacks = callbacks;
      if (!this.match()) {
        this.log.warn('页面不匹配，适配器未启动');
        return false;
      }
      // 记录当前域名，供持久化键（synced:channel:domain）区分账号
      try { this.domain = location && location.host ? location.host : ''; } catch (_) { this.domain = ''; }
      const meta = this.snapshotMeta();
      this.account = meta.accountId || this.account;
      this.conversationId = this.getConversationId();
      this._attachConversation();
      this._startConvPolling();
      // 私信全量遍历同步：自动同步所有会话（持久化续传，仅同步新增/未同步会话）
      this._loadSyncedConvIds();
      this._scheduleInitialSync();
      this._startRescan();
      // 巡检制度：自动启动（首轮在 intervalMs 后，确保存量历史已被首次同步以 history 落库、
      // 不会被巡检误当新消息触发 AI）。可由 popup 配置 intervalMs 或停止。
      this._startPatrolAuto();
      return true;
    } catch (e) {
      // 任何异常：记录详细错误，清理已分配资源（防止半初始化状态遗留 observer/timer），
      // 返回 false 让上层 sync() 跳过此适配器，其他渠道继续工作。
      this.log.error('适配器启动失败，已隔离异常', e);
      try { this.stop(); } catch (_) { /* stop 自身异常忽略，避免二次抛错 */ }
      return false;
    }
  }

  stop() {
    if (this.observer) this.observer.disconnect();
    if (this.convPollTimer) clearInterval(this.convPollTimer);
    if (this.fallbackTimer) clearInterval(this.fallbackTimer);
    if (this._graceTimer) clearTimeout(this._graceTimer);
    if (this._rescanTimer) clearInterval(this._rescanTimer);
    this._stopPatrol();
    this._convWindow.clear();
  }

  // 会话切换探测：切换后重挂载观察器并回填新会话历史
  //
  // 2026-08-05 修复（小红书日志无限打印 + 巡检失效）：
  //   原版每 2s 检查 cid 变化即触发 _attachConversation，但小红书 getConversationId
  //   兜底链（昵称派生 'conv:name'）在 DOM 异步渲染/未读数变化时会返回不同值 →
  //   每 2s 触发会话切换 → _backfill 打印日志 → 日志刷屏。
  //
  // 修复：去抖动——连续 2 次读取到相同 cid 才认为真的切换，避免 DOM 抖动导致的假切换。
  //   _pendingCid 记录上一次读取值；仅当连续两次相同时才提交切换。
  //   2s × 2 = 最迟 4s 检测到真实切换，可接受（用户手动点击切换会话也有渲染延迟）。
  _startConvPolling() {
    this._pendingCid = null;
    this.convPollTimer = setInterval(() => {
      const cid = this.getConversationId();
      if (!cid) { this._pendingCid = null; return; }
      if (cid === this.conversationId) { this._pendingCid = null; return; }
      // 去抖动：第一次读到新 cid 先暂存，第二次再确认
      if (this._pendingCid !== cid) {
        this._pendingCid = cid;
        return;
      }
      // 连续两次相同 → 确认切换
      this._pendingCid = null;
      this.log.info(`会话切换: ${this.conversationId} -> ${cid}`);
      this.conversationId = cid;
      this._attachConversation();
    }, 2000);
  }

  _attachConversation() {
    const root = this.getMessageRoot();
    this.activeRoot = root;
    if (root) {
      this._observe(root);
      this._backfill(root); // 初次/切换时立即回填（线程已渲染则生效）
    }
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
    // 修复（2026-08-05 OOM 巡检）：
    //   移除 characterData:true —— 抖音/小红书 IM 输入框抖屏/光标移动会产生成百上千次
    //   textNode 字符 mutation，叠加 subtree:true 全树监听会瞬间积压大量回调、撑爆
    //   MutationRecord 队列。文本类消息捕获走 childList 子树已足够（每条气泡本身
    //   就是 Element 节点变化，文本内容变化不携带 message_id 等结构信息）。
    //   配合 _onMutations 节流 100ms 合并批处理，单次最多处理 50 个新增元素，
    //   既保证实时性又防止抖屏 OOM。
    this.observer.observe(root, { childList: true, subtree: true });
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
    // 修复（2026-08-05 OOM 巡检）：批处理 + 节流 + 上限。
    // 抖音/小红书输入框光标移动、虚拟列表滚动会高频触发 mutation；
    // 100ms 内的所有 mutation 合并成一次处理，元素数封顶 50 条，
    // 超出部分丢弃（已 seen 的靠指纹去重，未 seen 的下轮扫描兜底），
    // 避免单次回调内存爆 + 抢占主线程卡顿。
    this._pendingMutations = (this._pendingMutations || []).concat(muts);
    if (this._mutationTimer) return;
    this._mutationTimer = setTimeout(() => {
      this._mutationTimer = null;
      const all = this._pendingMutations || [];
      this._pendingMutations = [];
      this._flushMutations(all);
    }, 100);
  }

  _flushMutations(muts) {
    // 用用户配置选择器优先 → SEL 默认兜底
    let itemSelectors = [];
    try {
      // 尝试从渠道 hooks 获取合并后的选择器（含用户配置）
      const selectors = this.hooks.mergedSelectors ? this.hooks.mergedSelectors() : null;
      if (selectors && selectors.itemSelectors && selectors.itemSelectors.length) {
        itemSelectors = selectors.itemSelectors;
      }
    } catch (_) { /* 忽略，回退 SEL */ }
    if (!itemSelectors.length) {
      itemSelectors = (this.SEL.MSG_ITEM || '').split(',').map((s) => s.trim()).filter(Boolean);
    }
    const nodes = this._addedElements(muts);
    // 上限：单次回调最多处理 50 个新增 DOM 节点（防抖屏 OOM）
    const MAX_PER_FLUSH = 50;
    let processed = 0;
    for (const node of nodes) {
      if (processed >= MAX_PER_FLUSH) break;
      let items = [];
      for (const sel of itemSelectors) {
        try {
          if (node.matches && node.matches(sel)) items.push(node);
          if (node.querySelectorAll) {
            const sub = node.querySelectorAll(sel);
            for (const s of sub) items.push(s);
          }
        } catch (_) { /* 非法选择器跳过 */ }
      }
      // 同一节点可能命中多个选择器，seenNodes 会在 _handleIncremental 内去重
      for (const item of items) {
        if (processed >= MAX_PER_FLUSH) break;
        this._handleIncremental(item);
        processed++;
      }
    }
  }

  // 兜底全量扫描（与去重结合，安全可重复调用）：纯 SEL 规则路径。
  _scanIncremental() {
    const items = this.getMessageItems();
    for (const item of items) this._handleIncremental(item);
  }

  // ============================================================
  // 私信会话全量遍历同步（核心能力）
  // 需求：① 一个私信=一个会话=统一收信中心一条记录；
  //       ② 遍历所有私信（≥99 人）逐个上报，而非只处理当前打开的那个；
  //       ③ 每个会话回填其多条历史消息（_backfill 已具备）。
  // 机制：枚举会话列表 → 逐个点击打开 → 等线程渲染 → 清空抽取缓存 → 回填历史。
  // 已同步会话写入持久化集合，刷新/重连后续传，仅同步新增会话。
  // ============================================================

  // 持久化键：按 渠道:域名 区分账号，避免 A 账号的已同步集合污染 B 账号。
  _syncedKey() {
    return `hivebridge:synced:${this.channel}:${this.domain || (typeof location !== 'undefined' ? location.host : '')}`;
  }

  _loadSyncedConvIds() {
    try {
      if (typeof localStorage === 'undefined') return;
      const raw = localStorage.getItem(this._syncedKey());
      if (!raw) return;
      const arr = JSON.parse(raw);
      if (Array.isArray(arr)) for (const id of arr) this._syncedConvIds.add(id);
    } catch (_) { /* localStorage 不可用或数据损坏，忽略 */ }
  }

  _saveSyncedConvIds() {
    try {
      if (typeof localStorage === 'undefined') return;
      localStorage.setItem(this._syncedKey(), JSON.stringify(Array.from(this._syncedConvIds)));
    } catch (_) { /* 配额/隐私模式忽略 */ }
  }

  // 首次激活后延迟调度一次全量同步（等列表与首个会话渲染就绪）
  // 2026-08-05 修复：throttleMs 1000 → 1500（用户诉求：会话间间隔 1-2 秒）
  _scheduleInitialSync() {
    if (this._initialSyncDone) return;
    this._initialSyncDone = true;
    setTimeout(() => {
      this.syncAllConversations({ throttleMs: 1500 }).catch(() => {});
    }, 8000);
  }

  // 周期重扫：捕获自动同步之后新增的私信会话（已同步的会被过滤，不会重复回填）。
  // 2026-08-05 修复：throttleMs 1000 → 1500（用户诉求：会话间间隔 1-2 秒）
  _startRescan() {
    if (this._rescanTimer) clearInterval(this._rescanTimer);
    this._rescanTimer = setInterval(() => {
      this.syncAllConversations({ throttleMs: 1500 }).catch(() => {});
    }, 10 * 60 * 1000);
  }

  // 等待会话切换：点击会话项后，活动会话 id 应从 prevCid 变为目标/其它值。
  // 返回：
  //   - targetId：活动会话已是目标（含「列表首项即当前打开会话」，点击前已就绪）
  //   - 其它非 prevCid 的 id：点击生效切到了别处（仍按成功回填当前线程）
  //   - null：超时未切换（点击未注册 / 非可点击元素）→ 调用方记失败
  // 关键：不能一有值就返回——SPA 异步渲染时 getConversationId 可能先返回旧值，
  // 若把它当 realCid 会误判「未打开线程」。必须等 cur !== prevCid。
  async _waitForActiveConversation(targetId, prevCid, timeout = 5000) {
    const start = Date.now();
    while (Date.now() - start < timeout) {
      const cur = this.getConversationId();
      if (cur === targetId) return cur; // 目标已打开（含原本就是目标）
      if (cur && cur !== prevCid) return cur; // 已切到其它会话
      await sleep(200);
    }
    return null;
  }

  // 在左侧会话列表找到目标会话并点击打开（上行遍历 / 下行回写共用）。
  // 返回打开后的活动会话 id；找不到目标 / 点击后未打开 → 返回 null。
  // 下行场景（AI 回复目标用户 ≠ 当前打开的会话）用它切到正确会话再发送。
  async openConversation(cid, { waitActiveMs = 5000, backfill = false } = {}) {
    if (!cid) return null;
    const cur = this.getConversationId();
    if (cur === cid) {
      // 已在该会话：若需要回填则执行（遍历器首项=当前会话场景，仍要回填历史）
      if (backfill) {
        this._backfill();
      }
      return cur;
    }
    let list = [];
    try {
      list = this.getConversationList() || [];
    } catch (_) { list = []; }
    if (!Array.isArray(list)) list = [];
    // 命中目标：优先精确 id；兼容 id 表述不一致（如 /user/ 链接 vs data-* / conv: 前缀）
    // 2026-08-05 修复：补全双向 conv: 前缀匹配。
    //   原版仅覆盖 cid 带 conv: 前缀（剥掉比对）和 c.id 不带但 cid 带（补 conv: 比对）两个方向，
    //   缺漏「c.id 带 conv: 前缀但 cid 不带」→ 巡检用 data-conv-id 找会话时找不到 → 巡检失效。
    let target = list.find((c) => c && c.id === cid);
    if (!target) {
      target = list.find((c) => c && (
        c.id === cid.replace(/^conv:/, '') ||
        ('conv:' + c.id) === cid ||
        c.id === 'conv:' + cid ||
        c.id.replace(/^conv:/, '') === cid
      ));
    }
    if (!target || !target.el) {
      this.log.warn(`左侧列表未找到目标会话 ${cid}`);
      return null;
    }
    const prevCid = this.getConversationId();
    // 滚入视口 + 模拟真实点击（兼容虚拟列表 & React 事件）
    if (typeof target.el.scrollIntoView === 'function') {
      try { target.el.scrollIntoView({ block: 'nearest' }); } catch (_) { /* 不可见元素忽略 */ }
    }
    try {
      if (target.el.tagName === 'A' || target.el.tagName === 'BUTTON') target.el.click();
      else simulateRealClick(target.el);
    } catch (_) { /* 点击异常忽略 */ }
    const opened = await this._waitForActiveConversation(cid, prevCid, waitActiveMs);
    if (!opened) {
      this.log.warn(`会话 ${cid} 点击后未打开线程`);
      return null;
    }
    this.log.info(`已切换到会话 ${opened}`);
    if (backfill) {
      this._backfill();
    }
    return opened;
  }

  // 找到会话列表的可滚动容器（用于加载更多被虚拟/懒加载的会话项）。
  _findListScroller() {
    try {
      const list = this.getConversationList();
      const probe = list && list[0] && list[0].el;
      if (!probe) return null;
      let cur = probe.parentElement;
      while (cur && cur !== document.body) {
        const style = (typeof getComputedStyle === 'function') ? getComputedStyle(cur) : null;
        const scrollable =
          style && (style.overflowY === 'auto' || style.overflowY === 'scroll' || cur.scrollHeight > cur.clientHeight + 50);
        if (scrollable) return cur;
        cur = cur.parentElement;
      }
    } catch (_) { /* noop */ }
    return null;
  }

  // 全量遍历所有私信会话并上报（每个会话=统一收信中心一条记录，含其历史消息）。
  // 多轮扫描：每轮结束后把会话列表滚动到底部，加载被虚拟/懒加载的更多项，直到无新增或达上限。
  // 返回 { synced, total, failures }。可经 popup / 开发工具手动触发：
  //   chrome.tabs.sendMessage(tabId, { type: 'syncAllConversations' })
  //
  // 2026-08-05 修复（用户诉求：遍历会话列表不允许太快，两个会话/两轮列表之间间隔 1-2 秒）：
  //   - throttleMs 默认 1000 → 1500（会话间节流，落在 1-2s 区间中值）
  //   - 新增 scrollLoadMs 参数（两轮列表扫描间隔），默认 1500ms（旧值硬编码 500ms 太快）
  async syncAllConversations({ max = 0, throttleMs = 1500, waitActiveMs = 5000, scrollLoadMs = 1500, onProgress } = {}) {
    if (this._syncingAll) {
      this.log.warn('会话全量同步已在进行，忽略重复触发');
      return { skipped: true, reason: 'in-progress' };
    }
    const hook = this.hooks.getConversationList;
    if (typeof hook !== 'function') {
      return { skipped: true, reason: 'no-hook' };
    }
    this._syncingAll = true;
    this.log.info('开始遍历同步私信会话（多轮扫描至列表底部）…');
    let done = 0;
    let failures = 0;
    let scannedTotal = 0;
    try {
      let pass = 0;
      const MAX_PASSES = 16;
      while (pass < MAX_PASSES) {
        pass++;
        let list = [];
        try {
          list = hook() || [];
        } catch (e) {
          this.log.error('读取会话列表失败', e);
          break;
        }
        if (!Array.isArray(list)) list = [];
        // 去重 + 跳过已同步（持久化续传）
        const seen = new Set();
        list = list.filter((c) => {
          if (!c || !c.id) return false;
          if (seen.has(c.id)) return false;
          seen.add(c.id);
          return true;
        });
        list = list.filter((c) => !this._syncedConvIds.has(c.id));
        if (max > 0 && list.length > max) list = list.slice(0, max);
        if (!list.length) break; // 本轮无新增 → 全部同步完
        for (const conv of list) {
          try {
            // 复用 openConversation：左侧列表找会话项 → 点击打开线程（兼容虚拟列表 & React 事件）。
            // 若活动会话已是目标（列表首项=当前打开会话）无需点击，直接回填。
            const opened = await this.openConversation(conv.id, { waitActiveMs, backfill: true });
            if (!opened) {
              this.log.warn(`会话 ${conv.id} 点击后未打开线程，跳过回填`, { name: conv.name });
              failures++;
              continue;
            }
            this._syncedConvIds.add(opened);
            // 兼容 id 表述不一致：列表 id（conv.id，常为 /user/<id> 链接）与活动会话 id
            // （opened，可能取 data-conversation-id）不同时，两者都记为已同步，
            // 否则下一轮/下次 rescan 仍按 conv.id 过滤 → 同一会话永不续传。
            if (conv.id && conv.id !== opened) this._syncedConvIds.add(conv.id);
            this._saveSyncedConvIds();
            done++;
            scannedTotal++;
            if (onProgress) {
              try { onProgress({ done, total: scannedTotal, id: conv.id, name: conv.name }); } catch (_) { /* noop */ }
            }
            await sleep(throttleMs);
          } catch (e) {
            this.log.error('同步单个私信会话异常', conv.id, e);
            failures++;
            continue;
          }
        }
        // 本轮结束：滚动列表到底部，等待加载更多（虚拟/懒加载列表），继续下一轮
        // 2026-08-05 修复：两轮列表之间间隔从 500ms 提到 scrollLoadMs（默认 1500ms），
        //   旧值 500ms 太快，虚拟列表还没加载完就开下一轮 → 漏抓 + 平台风控
        const scroller = this._findListScroller();
        if (scroller) {
          try { scroller.scrollTop = scroller.scrollHeight; } catch (_) { /* noop */ }
          await sleep(scrollLoadMs);
        } else {
          break; // 无列表容器可滚动 → 单轮即可覆盖全部
        }
      }
    } finally {
      this._syncingAll = false;
    }
    this.log.info(`私信会话遍历同步完成：成功 ${done}，失败 ${failures}`);
    return { synced: done, total: scannedTotal, failures };
  }

  // ============================================================
  // 巡检制度（patrol）—— 需求上行②
  // 一轮巡检：遍历左侧会话列表 → 对「有新消息」（未读红点）的会话点击进入右侧聊天页
  // → 捕获新消息（已 seen 的靠去重跳过）→ 一条会话级 inbound 帧上行（触发 AI 自动对话）。
  // 一轮结束自动进入下一轮；间隔参数可配置（PATROL_DEFAULTS 给默认值）。
  // 与全量同步(syncAllConversations)区别：同步把存量当 history 落库不触发 AI；
  //          巡检把「新增」当 inbound 上行触发 AI（自动对话），靠 seen 去重避免重复。
  // ============================================================

  // 自动启动巡检（使用 PATROL_DEFAULTS + 会话内缓存的 interval 覆盖）
  _startPatrolAuto() {
    const intervalMs = this._patrolIntervalFromConfig() || PATROL_DEFAULTS.intervalMs;
    this.startPatrol({ intervalMs });
  }

  // 读取会话内缓存的巡检间隔（popup 可改）；未配置返回 0
  _patrolIntervalFromConfig() {
    try {
      if (typeof localStorage === 'undefined') return 0;
      const v = localStorage.getItem(`hivebridge:patrol:${this.channel}`);
      if (!v) return 0;
      const n = parseInt(v, 10);
      return Number.isFinite(n) && n > 0 ? n : 0;
    } catch (_) { return 0; }
  }
  _savePatrolInterval(ms) {
    try {
      if (typeof localStorage === 'undefined') return;
      if (!ms || ms <= 0) localStorage.removeItem(`hivebridge:patrol:${this.channel}`);
      else localStorage.setItem(`hivebridge:patrol:${this.channel}`, String(Math.max(1000, Math.floor(ms))));
    } catch (_) { /* noop */ }
  }

  // 启动巡检循环：完成一轮后等待 intervalMs 再开始下一轮，永不停歇直到 stopPatrol
  startPatrol(opts = {}) {
    const intervalMs = opts.intervalMs && opts.intervalMs > 1000
      ? Math.floor(opts.intervalMs)
      : (this._patrolIntervalFromConfig() || PATROL_DEFAULTS.intervalMs);
    this._patrolOpts = {
      intervalMs,
      throttleMs: opts.throttleMs || PATROL_DEFAULTS.throttleMs,
      waitActiveMs: opts.waitActiveMs || PATROL_DEFAULTS.waitActiveMs,
      maxPerRound: opts.maxPerRound != null ? opts.maxPerRound : PATROL_DEFAULTS.maxPerRound,
      scrollLoadMs: opts.scrollLoadMs || PATROL_DEFAULTS.scrollLoadMs,
      maxPasses: opts.maxPasses || PATROL_DEFAULTS.maxPasses,
      visitAllWhenNoUnread: !!opts.visitAllWhenNoUnread,
    };
    this._savePatrolInterval(intervalMs);
    if (this._patrolTimer) clearTimeout(this._patrolTimer);
    // 2026-08-05 修复：降为 debug。start() 在 sync() 反复激活时会被多次调用，
    //   info 级别导致日志刷屏（配合 onDisconnect 紧密循环一分钟上万次）
    this.log.debug(`巡检已启动：每 ${intervalMs}ms 一轮`);
    this._scheduleNextPatrol(0);
    return { ok: true, intervalMs };
  }

  stopPatrol() {
    this._stopPatrol();
    this.log.debug('巡检已停止');
    return { ok: true };
  }

  _stopPatrol() {
    if (this._patrolTimer) { clearTimeout(this._patrolTimer); this._patrolTimer = null; }
    this._patrolOpts = null;
  }

  isPatrolling() { return !!this._patrolOpts; }

  patrolStatus() {
    return {
      running: !!this._patrolOpts,
      intervalMs: this._patrolOpts ? this._patrolOpts.intervalMs : 0,
      inRound: this._patrolling,
      ...this._patrolStats,
    };
  }

  _scheduleNextPatrol(delay) {
    if (!this._patrolOpts) return;
    this._patrolTimer = setTimeout(() => {
      // 防止与全量同步并发：syncAllConversations 进行中时本轮跳过
      if (this._syncingAll) { this._scheduleNextPatrol(this._patrolOpts.intervalMs); return; }
      this.patrol(this._patrolOpts)
        .catch((e) => this.log.warn('巡检轮异常', e && e.message))
        .finally(() => {
          if (this._patrolOpts) this._scheduleNextPatrol(this._patrolOpts.intervalMs);
        });
    }, Math.max(0, delay));
  }

  // 一轮巡检：遍历未读会话，逐个点击打开 → 捕获新消息 → 上行 inbound。
  // 返回 { visited, withNew, captured, failures }。
  //
  // 修复（2026-08-05 巡检机制优化）：
  //   两阶段扫描——第一阶段只读左侧会话列表（getConversationList）收集未读 id，
  //   第二阶段才逐个点开。旧版"列表遍历 + 即时点击"在虚拟列表下会触发大量滚动加载
  //   + 全部点开 → 几百个会话全部 visit、几十次 openConversation 抢占主线程，Chrome
  //   内存 / CPU 飙升。新版：
  //     1) scanOnly 模式：仅扫描列表，不点开任何会话（轻量级实时探测，用于「用户
  //        正在浏览时」实时检测未读变化，让 MutationObserver 自然捕获新消息）
  //     2) visit 模式：仅对 (扫描阶段产物) 未读会话点击，最大并发 MAX_VISIT_PARALLEL=2
  //     3) 用户已停留在某会话时：自动跳过点开，直接靠 MutationObserver / 3s 兜底扫描
  //        捕获新消息
  async patrol({
    throttleMs = PATROL_DEFAULTS.throttleMs,
    waitActiveMs = PATROL_DEFAULTS.waitActiveMs,
    maxPerRound = PATROL_DEFAULTS.maxPerRound,
    scrollLoadMs = PATROL_DEFAULTS.scrollLoadMs,
    maxPasses = PATROL_DEFAULTS.maxPasses,
    visitAllWhenNoUnread = false,
    scanOnly = false,
  } = {}) {
    if (this._patrolling) { this.log.warn('巡检已在进行，忽略重复触发'); return { skipped: true, reason: 'in-progress' }; }
    const hook = this.hooks.getConversationList;
    if (typeof hook !== 'function') { return { skipped: true, reason: 'no-hook' }; }
    this._patrolling = true;
    const startedAt = Date.now();
    let visited = 0, withNew = 0, captured = 0, failures = 0, scannedTotal = 0, unreadCount = 0;

    // 记住巡检前活动会话，巡检结束后尽量回到它（减少打扰）
    const beforeConv = this.getConversationId();

    try {
      // ============ 阶段 1：扫描左侧列表（轻量级）============
      // 2026-08-05 修复：巡检改为直接遍历所有会话，不再判断未读。
      //   原版仅访问有未读标记的会话（c.unread），但小红书未读徽章 DOM 不稳定
      //   （.xhs-im-conv-item__bottom-right 常为空 <!---->），detectUnread 全返回 false →
      //   "未读 0，跳过本轮" → 新消息永不上报。
      //   新逻辑：直接遍历所有会话，靠 _collectUnseenText 的 seenNodes/seen 去重，
      //   只有真正新增的消息才上行 inbound 触发 AI（"合理评论" = 不重复触发已处理消息）。
      const allList = [];
      try {
        const list = hook() || [];
        if (Array.isArray(list)) allList.push(...list);
      } catch (e) { this.log.warn('巡检阶段1读取列表失败', e && e.message); }
      // 去重
      const seenIds = new Set();
      const uniqList = allList.filter((c) => c && c.id && !seenIds.has(c.id) && seenIds.add(c.id));
      scannedTotal = uniqList.length;

      if (scanOnly) {
        // 仅扫描模式：返回统计，不进入任何会话
        this.log.debug(`巡检扫描-only 完成：总 ${scannedTotal}`);
        return { scannedTotal, unreadCount: 0, visited: 0, withNew: 0, captured: 0, failures: 0, durationMs: Date.now() - startedAt };
      }

      // 直接遍历所有会话，靠 seenNodes/seen 去重确保只捕获新消息
      let targets = uniqList;
      if (maxPerRound > 0 && targets.length > maxPerRound) targets = targets.slice(0, maxPerRound);

      // ============ 阶段 2：分批点开未读会话（限并发）============
      // MAX_VISIT_PARALLEL=2：同一时刻最多 2 个 _patrolVisit 跑（控制 DOM mutation 并发）
      const MAX_VISIT_PARALLEL = 2;
      const semaphore = new Array(MAX_VISIT_PARALLEL).fill(Promise.resolve());
      const nextSlot = () => {
        const p = semaphore.shift();
        semaphore.push(p);
        return p;
      };

      for (const conv of targets) {
        // 智能跳过：用户已经停留在该会话里 → 不点开（直接走 MutationObserver 捕获）
        if (this.getConversationId() === conv.id) {
          this.log.debug(`巡检跳过：用户已停留在会话 ${conv.id}（依赖 observer 实时捕获）`);
          continue;
        }
        try {
          // 限并发：每个 _patrolVisit 占一个 semaphore 槽位
          const prev = nextSlot();
          const visitPromise = prev.then(() => this._patrolVisit(conv, { throttleMs, waitActiveMs }));
          semaphore[semaphore.length - 1] = visitPromise.catch((e) => {
            this.log.warn('巡检单个会话异常', conv && conv.id, e && e.message);
            failures++;
          });
          const r = await visitPromise;
          visited++;
          if (r && r.ok && r.newCount > 0) { withNew++; captured += r.newCount; }
          await sleep(throttleMs);
        } catch (e) {
          this.log.warn('巡检单个会话异常', conv && conv.id, e && e.message);
          failures++;
        }
        if (this._patrolOpts && maxPerRound > 0 && visited >= maxPerRound) break;
      }
    } finally {
      this._patrolling = false;
    }
    // 尽量回到巡检前活动会话（减少打扰）
    if (beforeConv && beforeConv !== this.getConversationId()) {
      try { await this.openConversation(beforeConv, { backfill: false, waitActiveMs: 2000 }); } catch (_) { /* noop */ }
    }
    const dur = Date.now() - startedAt;
    this._patrolStats.rounds++;
    this._patrolStats.visited += visited;
    this._patrolStats.withNew += withNew;
    this._patrolStats.captured += captured;
    this._patrolStats.failures += failures;
    this._patrolStats.lastRoundAt = startedAt;
    this._patrolStats.lastDurationMs = dur;
    // 2026-08-05 修复：巡检一轮完成日志降为 debug（每 60s 一轮，info 刷屏）
    this.log.debug(`巡检一轮完成：扫描 ${scannedTotal} 个会话，访问 ${visited}，有新消息 ${withNew}，捕获新消息 ${captured}，失败 ${failures}，用时 ${dur}ms`);
    return { scannedTotal, unreadCount, visited, withNew, captured, failures, durationMs: dur };
  }

  // 巡检访问单个未读会话：点击打开（不触发 history 回填），等待渲染，收集未 seenNodes 的文字消息，
  // 一条会话级消息上行。已扫描的 DOM 节点靠 seenNodes 跳过 → 不重复打扰。
  //
  // 2026-08-05 修复（用户诉求：遍历会话列表不允许太快）：
  //   - 渲染等待（renderWaitMs）与节流（throttleMs）分离，职责清晰
  //   - renderWaitMs = min(throttleMs, 600)：DOM 渲染等待，封顶 600ms 防过长（技术性等待）
  //   - throttleMs（默认 1500ms）：会话间节流，在 patrol 主循环里 sleep（用户诉求 1-2s）
  //   - 旧注释「节流的一部分作为渲染等待」有歧义，已澄清
  async _patrolVisit(conv, { throttleMs, waitActiveMs }) {
    const opened = await this.openConversation(conv.id, { backfill: false, waitActiveMs });
    if (!opened) return { ok: false, newCount: 0 };
    // 等待线程异步渲染（纯技术性等待，与 patrol 主循环里的 throttleMs 节流分离）
    const renderWaitMs = Math.min(throttleMs, 600);
    await sleep(renderWaitMs);
    const batch = this._collectUnseenText();
    if (batch.length) this._emitPatrolMessage(opened, conv, batch);
    return { ok: true, newCount: batch.length };
  }

  // 扫描当前线程所有消息项，收集「未 seenNodes + 文字」的消息，并立即标记 seenNodes（防止后续
  // convPollTimer 触发的 _backfill / observer 重复上行同一条消息）。
  //
  // 修复（2026-08-05 OOM 巡检）：
  //   - 单次最多收集 MAX_BATCH_PER_PATROL=80 条消息，防止「一个超长会话一次抓几千条」OOM
  //   - 内容 hash 去重已交给后端统一收信中心（seen Set 已移除）
  _collectUnseenText() {
    const batch = [];
    const MAX_BATCH_PER_PATROL = 80;
    let items = [];
    try { items = this.getMessageItems() || []; } catch (_) { items = []; }
    for (const item of items) {
      if (batch.length >= MAX_BATCH_PER_PATROL) {
        this.log.warn(`单次巡检抓取消息数达上限 ${MAX_BATCH_PER_PATROL}，剩余靠下轮扫描补齐`);
        break;
      }
      if (!item || this.seenNodes.has(item)) continue;
      let parsed;
      try { parsed = this.parseMessageItem(item); } catch (_) { continue; }
      if (!parsed) continue;
      // 需求⑥：只上行文字消息
      if (parsed.msg_type && parsed.msg_type !== 'text') { this.seenNodes.add(item); continue; }
      if (!parsed.text) { this.seenNodes.add(item); continue; }
      this.seenNodes.add(item);
      batch.push(parsed);
    }
    return batch;
  }

  // 巡检捕获的新消息 → 一条会话级消息上行（需求③：一个会话=一条消息，
  // 昵称作为系统客户名称/发件人）。history[] 含本次巡检新捕获的多轮。
  _emitPatrolMessage(cid, conv, batch) {
    const history = batch.map((p) =>
      this._historyItem(p, p.sender_type === SENDER.CUSTOMER ? DIRECTION.INBOUND : DIRECTION.OUTBOUND)
    );
    const last = batch[batch.length - 1];
    const groupItem = [...batch].reverse().find((p) => p.is_group);
    const isGroup = !!groupItem;
    const groupId = (groupItem && groupItem.group_id) || '';
    const groupName = (groupItem && groupItem.group_name) || '';
    const senderName = (conv && conv.name) || batch.find((p) => p.sender_name)?.sender_name || '';
    // 2026-08-05 根因修复：sender_type 和 event_id 必须根据最后一条消息设置，不能固定为 CUSTOMER。
    //   原实现固定 sender_type=SENDER.CUSTOMER → 巡检时 self/AI 消息也被标记为 customer
    //   → 后端触发 AI → 无限循环。
    const lastSenderType = last.sender_type || SENDER.CUSTOMER;
    const msg = makeUnifiedMessage({
      channel: this.channel,
      account_id: this.getAccountId(),
      conversation_id: cid,
      sender_type: lastSenderType,
      sender_id: isGroup && groupId ? groupId : (last.sender_id || cid),
      sender_name: senderName,
      content: last.text || '',
      msg_type: 'text',
      timestamp: last.timestamp || Date.now(),
      event_id: last.message_id,
      is_group: isGroup,
      group_id: groupId,
      group_name: groupName,
      history,
    });
    this.log.info(`巡检上行消息: 会话 ${cid}（${history.length} 轮新消息）`);
    if (this.callbacks.onMessage) this.callbacks.onMessage(msg);
  }

  // _ingest：统一的「上行」逻辑（纯桥接）。
  // 2026-08-05 架构重构（用户诉求）：
  //   - Bridge 端不再判断是否需要 AI 回复、不再做内容指纹去重、不再区分 customer/self/agent 上报路径
  //   - 所有消息统一走 _emitMessage 上报服务端（在消息中标记 sender_type，供后端判断）
  //   - 节点级去重（seenNodes）保留：同一 DOM 节点不重复处理（DOM 扫描必需）
  //   - 内容 hash 去重 + 回复判断 → 服务端统一收信中心负责
  _ingest(parsed) {
    if (!parsed) return;
    // 需求⑥：当前只上行文字消息；图片/语音/视频/撤回/系统等非文字消息一律不上行。
    if (parsed.msg_type && parsed.msg_type !== 'text') return;
    if (!parsed.text) return;
    // 所有消息统一上报，由服务端根据 sender_type 判断是否入库 / 是否回复
    this._pushWindow(parsed);
    this._emitMessage(parsed);
  }

  // 维护当前会话最近 N 轮上下文窗口（供实时消息帧携带多轮历史）
  _pushWindow(parsed) {
    const cid = this.getConversationId() || this.conversationId || '_';
    if (!parsed) return;
    let w = this._convWindow.get(cid);
    if (!w) { w = []; this._convWindow.set(cid, w); }
    w.push(parsed);
    if (w.length > HISTORY_CONTEXT_WINDOW) w.splice(0, w.length - HISTORY_CONTEXT_WINDOW);
    // 会话数膨胀保护：超出上限清理最旧一半（同一页面打开的会话有限，防御异常）
    if (this._convWindow.size > 200) {
      const keys = Array.from(this._convWindow.keys()).slice(0, 100);
      for (const k of keys) this._convWindow.delete(k);
    }
  }

  // 读取某会话的最近 N 轮窗口（默认当前会话）
  _windowFor(cid) {
    const w = this._convWindow.get(cid || this.getConversationId() || this.conversationId || '_');
    return w ? w.slice(-HISTORY_CONTEXT_WINDOW) : [];
  }

  _handleIncremental(item) {
    // 节点级去重：同一 DOM 节点（无论被 MutationObserver 还是 3s 兜底扫描命中）只处理一次，
    // 从根本上杜绝「反复扫描 → 同名消息无限重复上行」。
    if (item && this.seenNodes.has(item)) return;
    // 无活动会话（私信列表视图）时不捕获消息
    if (!this.getConversationId()) return;
    const parsed = this.parseMessageItem(item);
    if (!parsed) return;
    // 2026-08-05 架构重构：所有消息统一走 _ingest → _emitMessage（不区分 customer/self/agent）
    if (item) this.seenNodes.add(item);
    this._ingest(parsed);
  }

  // 回填存量消息（页面加载 / 会话切换）：一个会话 = 一条会话级消息，内含全部多轮历史。
  // 纯 SEL 规则路径（LLM 抽取架构已移除）。
  _backfill(root) {
    // 无活动会话（私信列表视图）：不回填，避免把会话列表里的联系人昵称当历史消息上行。
    if (!this.getConversationId()) {
      this.log.info('回填跳过：当前无活动会话（私信列表视图）');
      return;
    }
    const batch = []; // { parsed, direction }：本次回填去重后新收集的消息（多轮历史）
    const items = this.getMessageItems();
    for (const item of items) {
      if (this.seenNodes.has(item)) continue;
      const parsed = this.parseMessageItem(item);
      if (!parsed) continue;
      // 需求⑥：只回填文字消息，非文字消息跳过
      if (parsed.msg_type && parsed.msg_type !== 'text') continue;
      if (!parsed.text) continue;
      this.seenNodes.add(item);
      // 2026-08-05 架构重构：仅靠 seenNodes 节点级去重。内容 hash 去重交给服务端。
      batch.push({ parsed, direction: parsed.sender_type === SENDER.CUSTOMER ? DIRECTION.INBOUND : DIRECTION.OUTBOUND });
    }
    // 2026-08-05 修复：日志节流——同一会话 5s 内只打印一次 info（防止端口断开重连
    //   导致 seenNodes 失效后反复回填刷屏）。batch 为空时降为 debug。
    if (batch.length) {
      const now = Date.now();
      const shouldLog = !this._lastBackfillLogAt || (now - this._lastBackfillLogAt > 5000);
      if (shouldLog) {
        this.log.info(`回填会话 ${this.conversationId}: ${items.length} 条消息（新增 ${batch.length}）`);
        this._lastBackfillLogAt = now;
      } else {
        this.log.debug(`回填会话 ${this.conversationId}: ${items.length} 条消息（新增 ${batch.length}）`);
      }
      this._emitConversationHistory(batch);
    } else {
      this.log.debug(`回填会话 ${this.conversationId}: ${items.length} 条消息（新增 0，全部已去重）`);
    }
  }

  // 单条历史项：把解析后的消息归一化为 history[] 数组元素（帧内多轮历史协议）
  _historyItem(parsed, direction) {
    const cid = this.getConversationId() || this.conversationId || '';
    const st = parsed.sender_type || SENDER.CUSTOMER;
    // 与 makeUnifiedMessage 一致的 sender_id 解析：群聊→群 id；客户→会话 id；自己/AI→账号 id。
    // 缺失会落成 channel:unknown，破坏统一收信中心按「对方」聚合。
    const senderId =
      parsed.sender_id ||
      (parsed.is_group && parsed.group_id ? parsed.group_id : (st === SENDER.CUSTOMER ? cid : this.getAccountId()));
    return {
      event_id: parsed.message_id || '',
      sender_type: st,
      sender_id: senderId,
      sender_name: parsed.sender_name || '',
      content: parsed.text || '',
      media_url: parsed.media_url || '',
      msg_type: parsed.msg_type || 'text',
      timestamp: parsed.timestamp || Date.now(),
      direction,
      is_group: !!parsed.is_group,
      group_id: parsed.group_id || '',
      group_name: parsed.group_name || '',
      receiver_id: direction === DIRECTION.OUTBOUND ? cid : (parsed.receiver_id || ''),
    };
  }

  // 会话级历史帧：一个会话 = 一条消息，message.history[] 含全部多轮（点3 核心）。
  // 统一走 onMessage 回调，服务端按 message.history 逐条落库。
  _emitConversationHistory(batch) {
    if (!batch || !batch.length) return;
    const last = batch[batch.length - 1];
    const cid = this.getConversationId() || this.conversationId || '_';
    // 会话级群聊元数据：取最近一条带群信息的历史项
    const groupItem = [...batch].reverse().find((b) => b.parsed && b.parsed.is_group);
    const isGroup = !!groupItem;
    const groupId = (groupItem && groupItem.parsed.group_id) || '';
    const groupName = (groupItem && groupItem.parsed.group_name) || '';
    const history = batch.map(({ parsed, direction }) => this._historyItem(parsed, direction));
    // 摘要方向：有客户消息 → inbound（主导），否则 outbound
    const summaryDirection = batch.some((b) => b.direction === DIRECTION.INBOUND) ? DIRECTION.INBOUND : DIRECTION.OUTBOUND;
    // 2026-08-05 根因修复：sender_type 和 event_id 必须根据最后一条消息设置，不能固定为 CUSTOMER。
    //   原实现固定 sender_type=SENDER.CUSTOMER → 即使最后一条是 self/AI 发送的 outbound，
    //   后端也收到 customer 消息 → 触发 AI → 无限循环。
    //   修复：sender_type 取最后一条消息的 sender_type；event_id 取其 message_id（稳定）。
    const lastSenderType = last.parsed.sender_type || SENDER.CUSTOMER;
    const msg = makeUnifiedMessage({
      channel: this.channel,
      account_id: this.getAccountId(),
      conversation_id: cid,
      sender_type: lastSenderType,
      sender_id: isGroup && groupId ? groupId : (last.parsed.sender_id || cid),
      content: last.parsed.text || '',
      media_url: last.parsed.media_url || '',
      msg_type: last.parsed.msg_type || 'text',
      timestamp: last.parsed.timestamp,
      event_id: last.parsed.message_id,
      direction: summaryDirection,
      is_group: isGroup,
      group_id: groupId,
      group_name: groupName,
      history,
    });
    this.log.debug(`回填会话 ${cid}: 1 条会话级消息（${history.length} 轮历史, ${summaryDirection}）`);
    if (this.callbacks.onMessage) this.callbacks.onMessage(msg);
  }

  // _emitMessage：统一的单条消息上报入口（纯桥接）。
  // 所有消息（customer / self / agent）都走此方法上报，携带该会话最近 N 轮历史作为上下文。
  // 不做任何业务判断（是否入库 / 是否回复由后端根据 sender_type 判断）。
  _emitMessage(parsed) {
    const cid = this.getConversationId();
    const windowMsgs = this._windowFor(cid);
    const msg = makeUnifiedMessage({
      channel: this.channel,
      account_id: this.getAccountId(),
      conversation_id: cid,
      sender_type: parsed.sender_type,
      sender_id: parsed.is_group && parsed.group_id ? parsed.group_id : (parsed.sender_id || ''),
      content: parsed.text,
      media_url: parsed.media_url || '',
      msg_type: parsed.msg_type || 'text',
      timestamp: parsed.timestamp,
      event_id: parsed.message_id,
      is_group: !!parsed.is_group,
      group_id: parsed.group_id || '',
      group_name: parsed.group_name || '',
      sender_name: parsed.sender_name || '',
      raw: parsed.raw || null,
      history: windowMsgs.map((m) => this._historyItem(m, m.sender_type === SENDER.CUSTOMER ? DIRECTION.INBOUND : DIRECTION.OUTBOUND)),
    });
    this.log.info('上行消息:', (msg.content || `[${msg.msg_type}]`).slice(0, 40));
    if (this.callbacks.onMessage) this.callbacks.onMessage(msg);
  }

  // 下行回写（带限速风控）：AI 回复经此回写到网页。
  // 返回 true 表示已回写；false 表示被风控拦截或失败。
  // targetConvId：目标会话（AI 回复要发给的用户）。若与当前打开的会话不同，
  // 先在左侧列表找到该用户点击进入右侧聊天页，再模拟输入发送（用户诉求：
  // “左侧列表找用户 → 点击进入右侧 → 模拟输入发送”）。
  // 2026-08-05 架构重构：sendOutbound 只做纯转发，不再调 _markRecentSelf（防回环交给后端）。
  async sendOutbound(text, targetConvId) {
    if (!text) { this.log.warn('回复内容为空，跳过'); return false; }
    const account = this.getAccountId();
    let conv = this.getConversationId();
    // 目标会话明确且不等于当前 → 先切到目标会话
    if (targetConvId && conv !== targetConvId) {
      const opened = await this.openConversation(targetConvId, { backfill: false });
      if (!opened) {
        this.log.warn(`下行目标会话 ${targetConvId} 未打开，放弃发送`, { current: conv });
        return false;
      }
      conv = this.getConversationId();
    }
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
      // 上报 outbound 消息（Mutation 抓到的同源气泡由后端通过 sender_type 去重）
      this._emitMessage(
        { sender_type: SENDER.AGENT, text, media_url: '', timestamp: Date.now(), message_id: `out-${Date.now()}` }
      );
    }
    return ok;
  }
}
