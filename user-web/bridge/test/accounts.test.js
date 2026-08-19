import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { configStore, normalizeAccounts } from '../src/core/config-store.js';

// 与 background.test.js 一致的 chrome mock（含 storage.local + runtime.onMessage）
function makeChrome({ storage = {} } = {}) {
  const listeners = { message: [] };
  const data = { ...storage };
  let _lastError = null;
  const chrome = {
    runtime: {
      get lastError() { return _lastError; },
      set lastError(v) { _lastError = v; },
      onMessage: { addListener: (fn) => listeners.message.push(fn) },
    },
    storage: {
      local: {
        get: (key, cb) => {
          const out = {};
          if (typeof key === 'string') out[key] = data[key];
          else if (Array.isArray(key)) key.forEach((k) => (out[k] = data[k]));
          else Object.keys(data).forEach((k) => (out[k] = data[k]));
          cb(out);
        },
        set: (obj, cb) => {
          Object.assign(data, obj);
          cb && cb();
        },
      },
    },
    webNavigation: { onCommitted: { addListener: () => {} } },
    scripting: {},
    tabs: {
      query: vi.fn((q, cb) => cb && cb([{ id: 1, url: 'https://www.douyin.com/message' }])),
      sendMessage: vi.fn((id, msg, cb) => { cb && cb({ ok: true }); }),
    },
  };
  return { chrome, listeners, data, setLastError: (v) => { _lastError = v; } };
}

let currentMock = null;
beforeEach(() => {
  currentMock = makeChrome();
  globalThis.chrome = currentMock.chrome;
  configStore.__reset();
});
afterEach(() => {
  delete globalThis.chrome;
  currentMock = null;
});

describe('accounts normalizeAccounts', () => {
  it('null / undefined / 非对象 → {}', () => {
    expect(normalizeAccounts(undefined)).toEqual({});
    expect(normalizeAccounts(null)).toEqual({});
    expect(normalizeAccounts('x')).toEqual({});
    expect(normalizeAccounts([])).toEqual({});
    expect(normalizeAccounts(123)).toEqual({});
  });

  it('规范化补全 enabled/pausedAt', () => {
    expect(normalizeAccounts({ douyin: { a1: {} } })).toEqual({ douyin: { a1: { enabled: true, pausedAt: null } } });
    expect(normalizeAccounts({ douyin: { a1: { enabled: false, pausedAt: 5 } } })).toEqual({ douyin: { a1: { enabled: false, pausedAt: 5 } } });
  });

  it('非法子对象跳过', () => {
    expect(normalizeAccounts({ douyin: 123, xhs: null })).toEqual({});
  });
});

describe('accounts configStore 启停状态（P1-P6）', () => {
  it('默认全部启用（无 accounts 字段，向后兼容）', () => {
    expect(configStore.isAccountEnabled('douyin', 'a1')).toBe(true);
    expect(configStore.getAccountStates()).toEqual({});
    expect(configStore.getConfig().accounts).toEqual({});
  });

  it('暂停某账号后 isAccountEnabled=false，pausedAt 有值', async () => {
    await configStore.setAccountEnabled('douyin', 'a1', false);
    expect(configStore.isAccountEnabled('douyin', 'a1')).toBe(false);
    const st = configStore.getAccountStates();
    expect(st.douyin.a1.enabled).toBe(false);
    expect(typeof st.douyin.a1.pausedAt).toBe('number');
    expect(configStore.isAccountEnabled('douyin', 'a2')).toBe(true);
    expect(configStore.isAccountEnabled('xhs', 'a1')).toBe(true);
  });

  it('恢复后 enabled=true，pausedAt 清空', async () => {
    await configStore.setAccountEnabled('douyin', 'a1', false);
    await configStore.setAccountEnabled('douyin', 'a1', true);
    expect(configStore.isAccountEnabled('douyin', 'a1')).toBe(true);
    expect(configStore.getAccountStates().douyin.a1.pausedAt).toBeNull();
  });

  it('setAccountEnabled 持久化到 chrome.storage', async () => {
    const setSpy = vi.fn().mockResolvedValue();
    globalThis.chrome = { storage: { local: { get: vi.fn().mockResolvedValue({}), set: setSpy } } };
    await configStore.setAccountEnabled('xiaohongshu', 'a2', false);
    expect(setSpy).toHaveBeenCalled();
    const arg = setSpy.mock.calls[0][0];
    expect(arg.bridgeConfig.accounts.xiaohongshu.a2.enabled).toBe(false);
  });

  it('setAccountEnabled 触发 change 事件', async () => {
    const events = [];
    configStore.on('change', (n) => events.push(n));
    await configStore.setAccountEnabled('douyin', 'a1', false);
    expect(events.length).toBe(1);
    expect(events[0].accounts.douyin.a1.enabled).toBe(false);
  });

  it('缺少 channel/accountId → 抛错', async () => {
    await expect(configStore.setAccountEnabled('', 'a1', false)).rejects.toThrow('channel');
    await expect(configStore.setAccountEnabled('douyin', '', false)).rejects.toThrow('accountId');
  });
});

describe('accounts background 消息处理（bridge:setAccountEnabled / bridge:switchAccount）', () => {
  async function loadBackground() {
    vi.resetModules();
    await import('../src/background/index.js');
    return currentMock;
  }

  // 尝试所有监听器，找到能处理该消息的那个
  function callListener(listeners, msg, sendResponse) {
    let called = false;
    const wrapper = (resp) => { called = true; sendResponse(resp); };
    for (const fn of listeners.message) {
      const ret = fn(msg, {}, wrapper);
      if (ret !== false || called) return called;
    }
    return called;
  }

  it('bridge:setAccountEnabled 持久化到 config.accounts 并返回 ok', async () => {
    await loadBackground();
    const { listeners, data } = currentMock;
    const resp = await new Promise((resolve) => {
      callListener(listeners, { type: 'bridge:setAccountEnabled', channel: 'douyin', accountId: 'a1', enabled: false }, resolve);
    });
    expect(resp.ok).toBe(true);
    expect(data.bridgeConfig.accounts.douyin.a1.enabled).toBe(false);
    expect(typeof data.bridgeConfig.accounts.douyin.a1.pausedAt).toBe('number');
  });

  it('bridge:setAccountEnabled 启用 → pausedAt 为 null', async () => {
    await loadBackground();
    const { listeners, data } = currentMock;
    await new Promise((resolve) => {
      callListener(listeners, { type: 'bridge:setAccountEnabled', channel: 'douyin', accountId: 'a1', enabled: false }, resolve);
    });
    await new Promise((resolve) => {
      callListener(listeners, { type: 'bridge:setAccountEnabled', channel: 'douyin', accountId: 'a1', enabled: true }, resolve);
    });
    expect(data.bridgeConfig.accounts.douyin.a1.enabled).toBe(true);
    expect(data.bridgeConfig.accounts.douyin.a1.pausedAt).toBeNull();
  });

  it('bridge:setAccountEnabled 缺 channel → 返回 {ok:false}', async () => {
    await loadBackground();
    const { listeners } = currentMock;
    const resp = await new Promise((resolve) => {
      callListener(listeners, { type: 'bridge:setAccountEnabled', accountId: 'a1', enabled: false }, resolve);
    });
    expect(resp.ok).toBe(false);
    expect(resp.error).toContain('channel');
  });

  it('bridge:setAccountEnabled 广播 accountsUpdated 到 tabs', async () => {
    await loadBackground();
    const { listeners } = currentMock;
    const sendSpy = currentMock.chrome.tabs.sendMessage;
    await new Promise((resolve) => {
      callListener(listeners, { type: 'bridge:setAccountEnabled', channel: 'douyin', accountId: 'a1', enabled: false }, resolve);
    });
    expect(sendSpy).toHaveBeenCalled();
    const broadcastMsg = sendSpy.mock.calls[0][1];
    expect(broadcastMsg.type).toBe('bridge:accountsUpdated');
    expect(broadcastMsg.channel).toBe('douyin');
    expect(broadcastMsg.accountId).toBe('a1');
  });

  it('bridge:switchAccount 更新 channel/accountId/conversationId 并返回 ok', async () => {
    await loadBackground();
    const { listeners, data } = currentMock;
    const resp = await new Promise((resolve) => {
      callListener(listeners, { type: 'bridge:switchAccount', channel: 'douyin', accountId: 'a2', conversationId: 'c2' }, resolve);
    });
    expect(resp.ok).toBe(true);
    expect(data.bridgeConfig.channel).toBe('douyin');
    expect(data.bridgeConfig.accountId).toBe('a2');
    expect(data.bridgeConfig.conversationId).toBe('c2');
  });

  it('bridge:switchAccount 广播 accountsUpdated', async () => {
    await loadBackground();
    const { listeners } = currentMock;
    const sendSpy = currentMock.chrome.tabs.sendMessage;
    await new Promise((resolve) => {
      callListener(listeners, { type: 'bridge:switchAccount', channel: 'xhs', accountId: 'a3' }, resolve);
    });
    expect(sendSpy).toHaveBeenCalled();
    expect(sendSpy.mock.calls[0][1].type).toBe('bridge:accountsUpdated');
  });
});

describe('accounts channel-adapter 暂停/恢复影响巡检（P1-P6）', () => {
  it('暂停后停止巡检；patrol() 返回 account-paused', async () => {
    const { BaseAdapter } = await import('../src/core/channel-adapter.js');
    const { CHANNELS } = await import('../src/core/types.js');
    const adapter = new BaseAdapter({
      name: 'test',
      channel: CHANNELS.DOUYIN,
      SEL: {},
      hooks: { match: () => true, getConversationList: () => [] },
    });
    adapter.startPatrol({ intervalMs: 60000 });
    expect(adapter.isPatrolling()).toBe(true);

    adapter.setAccountEnabled(false);
    expect(adapter.isPatrolling()).toBe(false);
    expect(adapter.isAccountEnabled()).toBe(false);
    const r = await adapter.patrol();
    expect(r.skipped).toBe(true);
    expect(r.reason).toBe('account-paused');

    adapter.setAccountEnabled(true);
    expect(adapter.isAccountEnabled()).toBe(true);
  });

  it('暂停时 _startPatrolAuto 不启动；恢复后自动重开', async () => {
    const { BaseAdapter } = await import('../src/core/channel-adapter.js');
    const { CHANNELS } = await import('../src/core/types.js');
    const adapter = new BaseAdapter({
      name: 'test',
      channel: CHANNELS.DOUYIN,
      SEL: {},
      hooks: { match: () => true, getConversationList: () => [] },
    });
    adapter.setAccountEnabled(false);
    adapter._startPatrolAuto();
    expect(adapter.isPatrolling()).toBe(false);
    adapter.setAccountEnabled(true);
    expect(adapter.isPatrolling()).toBe(true);
  });
});

describe('accounts popup accounts.js', () => {
  it('renderAccountRows 渲染启停开关（暂停账号 → 启用按钮）', async () => {
    const { renderAccountRows } = await import('../src/popup/accounts.js');
    const html = renderAccountRows({ douyin: { acc1: { enabled: false, pausedAt: Date.now() } } });
    expect(html).toContain('抖音');
    expect(html).toContain('data-channel="douyin"');
    expect(html).toContain('data-account="acc1"');
    expect(html).toContain('data-enable="true"');
    expect(html).toContain('dot paused');
  });

  it('renderAccountRows 启用账号 → 暂停按钮', async () => {
    const { renderAccountRows } = await import('../src/popup/accounts.js');
    const html = renderAccountRows({ douyin: { acc1: { enabled: true } } });
    expect(html).toContain('data-enable="false"');
    expect(html).toContain('dot enabled');
  });

  it('renderAccountRows 多渠道按顺序排序（抖音 < 闲鱼）', async () => {
    const { renderAccountRows } = await import('../src/popup/accounts.js');
    const html = renderAccountRows({
      xianyu: { b1: { enabled: true } },
      douyin: { a1: { enabled: true } },
    });
    expect(html.indexOf('抖音')).toBeLessThan(html.indexOf('闲鱼'));
  });

  it('renderAccountRows 空账号 → 占位文案', async () => {
    const { renderAccountRows } = await import('../src/popup/accounts.js');
    expect(renderAccountRows({})).toContain('暂无已配置账号');
    expect(renderAccountRows(null)).toContain('暂无已配置账号');
  });

  it('XSS 转义：账号 ID 含尖括号不注入 HTML', async () => {
    const { renderAccountRows } = await import('../src/popup/accounts.js');
    const html = renderAccountRows({ douyin: { '<img src=x onerror=1>': { enabled: true } } });
    expect(html).not.toContain('<img');
    expect(html).toContain('&lt;img');
  });

  it('setAccountEnabledViaBackground 走 background 消息', async () => {
    const { setAccountEnabledViaBackground } = await import('../src/popup/accounts.js');
    const sendMessage = vi.fn((msg, cb) => {
      expect(msg.type).toBe('bridge:setAccountEnabled');
      expect(msg.channel).toBe('douyin');
      expect(msg.accountId).toBe('a1');
      expect(msg.enabled).toBe(false);
      cb({ ok: true });
    });
    globalThis.chrome = { runtime: { sendMessage } };
    const r = await setAccountEnabledViaBackground('douyin', 'a1', false);
    expect(r.ok).toBe(true);
  });

  it('setAccountEnabledViaBackground background 不可达 → 兜底直写 configStore', async () => {
    const { setAccountEnabledViaBackground } = await import('../src/popup/accounts.js');
    // 背景：上方 background describe 调过 vi.resetModules()，静态导入的 configStore 实例已过期；
    // 动态导入拿到的实例才与 accounts.js 内部共享，确保断言命中同一实例。
    const store = (await import('../src/core/config-store.js')).configStore;
    store.__reset();
    delete globalThis.chrome;
    const r = await setAccountEnabledViaBackground('douyin', 'a1', false);
    expect(r.ok).toBe(true);
    expect(r.fallback).toBe(true);
    expect(store.isAccountEnabled('douyin', 'a1')).toBe(false);
  });

  it('setAccountEnabledViaBackground background 返回失败 → 透传错误', async () => {
    const { setAccountEnabledViaBackground } = await import('../src/popup/accounts.js');
    globalThis.chrome = { runtime: { sendMessage: vi.fn((msg, cb) => cb({ ok: false, error: 'bg-boom' })) } };
    const r = await setAccountEnabledViaBackground('douyin', 'a1', false);
    expect(r.ok).toBe(false);
    expect(r.error).toContain('bg-boom');
  });

  it('fetchAccountStates 无 chrome → 回退 configStore 内存状态', async () => {
    const { fetchAccountStates } = await import('../src/popup/accounts.js');
    const store = (await import('../src/core/config-store.js')).configStore;
    delete globalThis.chrome;
    store.__reset();
    await store.setAccountEnabled('douyin', 'a1', false);
    const st = await fetchAccountStates();
    expect(st.douyin.a1.enabled).toBe(false);
  });
});

