
import { createLogger } from './logger.js';
import { PATROL_DEFAULTS, RATE_LIMIT_DEFAULTS } from './constants.js';

const log = createLogger('config', 'bridge');

const STORAGE_KEY = 'bridgeConfig';
const POLL_INTERVAL_MS = 30000;
const CURRENT_VERSION = 1;

// 账号启停状态规范化：{ channel: { accountId: { enabled: bool, pausedAt: number|null } } }
// 旧版本配置可能没有 accounts 字段，或某渠道/账号缺 enabled —— 一律补全为「默认启用」。
export function normalizeAccounts(accounts) {
  if (!accounts || typeof accounts !== 'object' || Array.isArray(accounts)) return {};
  const out = {};
  for (const channel of Object.keys(accounts)) {
    const ch = accounts[channel];
    if (!ch || typeof ch !== 'object' || Array.isArray(ch)) continue;
    const norm = {};
    for (const accountId of Object.keys(ch)) {
      const st = ch[accountId] && typeof ch[accountId] === 'object' ? ch[accountId] : {};
      norm[accountId] = {
        enabled: st.enabled !== false,
        pausedAt: typeof st.pausedAt === 'number' ? st.pausedAt : null,
      };
    }
    if (Object.keys(norm).length) out[channel] = norm;
  }
  return out;
}

// 默认配置（与 constants.js PATROL_DEFAULTS / RATE_LIMIT_DEFAULTS 对齐）
function buildDefaultConfig() {
  return {
    serverUrl: 'http://localhost:8204',
    token: '',
    channel: '',
    accountId: '',
    conversationId: '',
    patrol: { ...PATROL_DEFAULTS },
    rate: { ...RATE_LIMIT_DEFAULTS },
    circuit: { failureThreshold: 5, openDurationMs: 30000, deadManSeconds: 300 },
    accounts: {},
    version: CURRENT_VERSION,
    updatedAt: 0,
  };
}

class ConfigStore extends EventTarget {
  constructor() {
    super();
    this._cfg = buildDefaultConfig();
    this._loaded = false;
    this._listeners = new Set();
    this._pollTimer = null;
    this._initialized = false;
  }

  async init() {
    if (this._initialized) return;
    this._initialized = true;
    await this._loadFromStorage();
    this._attachStorageListener();
    this._startPolling();
    log.info(`config-store 初始化完成，version=${this._cfg.version}`);
  }

  async _loadFromStorage() {
    try {
      if (typeof chrome !== 'undefined' && chrome.storage && chrome.storage.local) {
        const r = await chrome.storage.local.get(STORAGE_KEY);
        const persisted = r && r[STORAGE_KEY];
        if (persisted && typeof persisted === 'object') {
          const merged = this._mergeWithDefault(persisted);
          this._cfg = merged;
          this._loaded = true;
        }
      }
    } catch (e) {
      log.warn('config-store 加载失败，使用默认值', e && e.message);
    }
    this._loaded = true;
  }

  _attachStorageListener() {
    if (typeof chrome === 'undefined' || !chrome.storage || !chrome.storage.onChanged) return;
    chrome.storage.onChanged.addListener((changes, area) => {
      if (area !== 'local' || !changes[STORAGE_KEY]) return;
      const newRaw = changes[STORAGE_KEY].newValue;
      if (!newRaw) return;
      const oldCfg = this._cfg;
      const merged = this._mergeWithDefault(newRaw);
      this._cfg = merged;
      log.info(`config-store 收到 onChanged 事件，version ${oldCfg.version} → ${merged.version}`);
      this._emit('change', merged, oldCfg);
    });
  }

  _startPolling() {
    if (this._pollTimer) return;
    if (typeof setInterval === 'undefined') return;
    this._pollTimer = setInterval(() => this._loadFromStorage().then(() => {
    }), POLL_INTERVAL_MS);
  }

  stop() {
    if (this._pollTimer) {
      clearInterval(this._pollTimer);
      this._pollTimer = null;
    }
  }

  _mergeWithDefault(persisted) {
    const def = buildDefaultConfig();
    return {
      ...def,
      ...persisted,
      patrol: { ...def.patrol, ...(persisted.patrol || {}) },
      rate:   { ...def.rate,   ...(persisted.rate || {}) },
      circuit: { ...def.circuit, ...(persisted.circuit || {}) },
      accounts: normalizeAccounts(persisted.accounts),
      version: persisted.version || def.version,
      updatedAt: persisted.updatedAt || 0,
    };
  }

  getConfig() { return this._cfg; }

  getAccountStates() {
    return normalizeAccounts(this._cfg.accounts);
  }

  isAccountEnabled(channel, accountId) {
    const st = this.getAccountStates();
    const ch = st[channel] || {};
    const acct = ch[accountId] || {};
    return acct.enabled !== false;
  }

  async setAccountEnabled(channel, accountId, enabled) {
    if (!channel || !accountId) throw new Error('setAccountEnabled 缺少 channel/accountId');
    const states = this.getAccountStates();
    const ch = states[channel] || {};
    ch[accountId] = {
      enabled: !!enabled,
      pausedAt: enabled ? null : Date.now(),
    };
    states[channel] = ch;
    await this.set({ accounts: states });
    return this.getAccountStates();
  }

  get(path, fallback) {
    if (!path) return fallback;
    const parts = path.split('.');
    let v = this._cfg;
    for (const p of parts) {
      if (v == null || typeof v !== 'object') return fallback;
      v = v[p];
    }
    return v == null ? fallback : v;
  }

  async set(patch) {
    const old = this._cfg;
    const next = this._mergeWithDefault({
      ...old,
      ...patch,
      version: (old.version || CURRENT_VERSION) + 1,
      updatedAt: Date.now(),
    });
    this._cfg = next;
    try {
      if (typeof chrome !== 'undefined' && chrome.storage && chrome.storage.local) {
        await chrome.storage.local.set({ [STORAGE_KEY]: next });
      }
    } catch (e) {
      log.warn('config-store 持久化失败', e && e.message);
    }
    this._emit('change', next, old);
    return next;
  }

  on(type, fn) {
    if (type !== 'change' || typeof fn !== 'function') return;
    this.addEventListener('change', (e) => fn(e.detail.newCfg, e.detail.oldCfg));
  }

  _emit(type, newCfg, oldCfg) {
    this.dispatchEvent(new CustomEvent(type, { detail: { newCfg, oldCfg } }));
  }

  __reset() {
    this._cfg = buildDefaultConfig();
    this._loaded = false;
  }
}

export const configStore = new ConfigStore();
export { buildDefaultConfig, STORAGE_KEY };


