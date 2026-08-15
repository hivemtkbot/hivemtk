import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
  CONFIG_FILE_NAME,
  CONFIG_SCHEMA,
  serializeConfig,
  encryptJSON,
  decryptJSON,
  exportConfig,
  importConfig,
  downloadConfigJson,
  getSubtle,
} from '../src/popup/config-io.js';
import { configStore } from '../src/core/config-store.js';

// Node >= 20 自带 globalThis.crypto.subtle；测试环境不可用时自动跳过真实加解密用例。
const hasSubtle = () => !!(globalThis.crypto && globalThis.crypto.subtle);

describe('config-io serializeConfig（导出序列化）', () => {
  it('包含全部字段：serverUrl/token/channel/accountId/conversationId/patrol/rate/circuit/version/accounts', () => {
    const out = serializeConfig({
      serverUrl: 'http://srv:8204',
      token: 'tk-secret',
      channel: 'douyin',
      accountId: 'acc1',
      conversationId: 'conv1',
      patrol: { intervalMs: 30000 },
      rate: { accountCapacity: 12 },
      circuit: { failureThreshold: 5 },
      version: 3,
      accounts: { douyin: { acc1: { enabled: false, pausedAt: 123 } } },
    });
    expect(out.schema).toBe(CONFIG_SCHEMA);
    expect(out.serverUrl).toBe('http://srv:8204');
    expect(out.token).toBe('tk-secret');
    expect(out.channel).toBe('douyin');
    expect(out.accountId).toBe('acc1');
    expect(out.conversationId).toBe('conv1');
    expect(out.patrol.intervalMs).toBe(30000);
    expect(out.rate.accountCapacity).toBe(12);
    expect(out.circuit.failureThreshold).toBe(5);
    expect(out.version).toBe(3);
    expect(out.accounts.douyin.acc1.enabled).toBe(false);
  });

  it('缺失字段补空串 / 空对象，不抛异常', () => {
    const out = serializeConfig({});
    expect(out.serverUrl).toBe('');
    expect(out.token).toBe('');
    expect(out.accounts).toEqual({});
    expect(out.patrol).toEqual({});
    expect(out.rate).toEqual({});
  });

  it('子对象拷贝，不引用原对象（防后续修改污染导出）', () => {
    const patrol = { intervalMs: 1000 };
    const out = serializeConfig({ patrol });
    patrol.intervalMs = 9999;
    expect(out.patrol.intervalMs).toBe(1000);
  });
});

describe('config-io 加密导出 / 解密导入（真实 Web Crypto）', () => {
  const skip = () => hasSubtle() ? false : true;

  it('加密导出结构完整（schema/encrypted/kdf/nonce/ciphertext）', async () => {
    if (skip()) return;
    const plain = { serverUrl: 'http://x:8204', token: 'secret-token', channel: 'douyin' };
    const payload = await encryptJSON(plain, 'pass-123');
    expect(payload.schema).toBe(CONFIG_SCHEMA);
    expect(payload.encrypted).toBe(true);
    expect(payload.kdf.algo).toBe('PBKDF2');
    expect(payload.kdf.salt).toBeTruthy();
    expect(payload.kdf.iterations).toBeGreaterThan(0);
    expect(payload.nonce).toBeTruthy();
    expect(payload.ciphertext).toBeTruthy();
  });

  it('加密导出 / 解密导入往返一致', async () => {
    if (skip()) return;
    const plain = { serverUrl: 'http://s:8204', token: 'tok', channel: 'xiaohongshu', patrol: { intervalMs: 5000 } };
    const payload = await encryptJSON(plain, 'pass-123');
    const back = await decryptJSON(payload, 'pass-123');
    expect(back).toEqual(plain);
  });

  it('同一明文不同随机盐 → 密文不同（随机性）', async () => {
    if (skip()) return;
    const plain = { token: 'same' };
    const p1 = await encryptJSON(plain, 'pass');
    const p2 = await encryptJSON(plain, 'pass');
    expect(p1.ciphertext).not.toBe(p2.ciphertext);
    expect(p1.kdf.salt).not.toBe(p2.kdf.salt);
  });

  it('错误口令解密失败（GCM 校验失败）', async () => {
    if (skip()) return;
    const payload = await encryptJSON({ token: 'x' }, 'right-pass');
    await expect(decryptJSON(payload, 'wrong-pass')).rejects.toThrow('口令错误');
  });

  it('空口令解密加密文件也失败', async () => {
    if (skip()) return;
    const payload = await encryptJSON({ token: 'x' }, 'right-pass');
    await expect(decryptJSON(payload, '')).rejects.toThrow('口令错误');
  });

  it('缺 kdf/nonce/ciphertext → 报字段缺失', async () => {
    if (skip()) return;
    await expect(decryptJSON({ encrypted: true, kdf: {} }, 'p')).rejects.toThrow('字段缺失');
  });
});

describe('config-io 明文结构 / 非法输入', () => {
  it('明文结构 { encrypted:false, data } 直接返回 data', async () => {
    const data = { serverUrl: 'http://x', token: 't' };
    const back = await decryptJSON({ schema: 1, encrypted: false, data }, '');
    expect(back).toEqual(data);
  });

  it('明文结构缺 data → 报错', async () => {
    await expect(decryptJSON({ schema: 1, encrypted: false }, '')).rejects.toThrow('缺少 data');
  });

  it('null / 非对象 payload → 报无效配置文件', async () => {
    await expect(decryptJSON(null, '')).rejects.toThrow('无效的配置文件');
    await expect(decryptJSON('str', '')).rejects.toThrow('无效的配置文件');
  });
});

describe('config-io exportConfig', () => {
  beforeEach(() => { configStore.__reset(); });

  it('无口令 → 明文导出，包含当前配置全部字段', async () => {
    await configStore.set({
      serverUrl: 'http://s:8204',
      token: 't-1',
      channel: 'douyin',
      accountId: 'a1',
      conversationId: 'c1',
      patrol: { intervalMs: 12345 },
    });
    const r = await exportConfig('', { store: configStore });
    expect(r.filename).toBe(CONFIG_FILE_NAME);
    expect(r.payload.encrypted).toBe(false);
    expect(r.payload.data.serverUrl).toBe('http://s:8204');
    expect(r.payload.data.token).toBe('t-1');
    expect(r.payload.data.patrol.intervalMs).toBe(12345);
    // text 是合法 JSON 且可被 JSON.parse 还原
    const parsed = JSON.parse(r.text);
    expect(parsed.data.accountId).toBe('a1');
  });

  it('带口令 → 加密导出（encrypted:true）', async () => {
    if (!hasSubtle()) return;
    await configStore.set({ token: 'secret' });
    const r = await exportConfig('passphrase', { store: configStore });
    expect(r.payload.encrypted).toBe(true);
    expect(r.payload.ciphertext).toBeTruthy();
    expect(r.text).not.toContain('secret');
  });

  it('空白口令按明文处理', async () => {
    await configStore.set({ token: 'x' });
    const r = await exportConfig('   ', { store: configStore });
    expect(r.payload.encrypted).toBe(false);
  });
});

describe('config-io importConfig（热更新）', () => {
  beforeEach(() => { configStore.__reset(); });

  it('明文 JSON 导入 → configStore 热更新（serverUrl/token/渠道/巡检）', async () => {
    const text = JSON.stringify({
      schema: 1,
      encrypted: false,
      data: {
        serverUrl: 'http://new:8204',
        token: 'new-token',
        channel: 'xiaohongshu',
        accountId: 'a2',
        patrol: { intervalMs: 7000 },
        accounts: { xiaohongshu: { a2: { enabled: false, pausedAt: 1 } } },
      },
    });
    const r = await importConfig(text, '');
    expect(r.ok).toBe(true);
    expect(configStore.get('serverUrl')).toBe('http://new:8204');
    expect(configStore.get('token')).toBe('new-token');
    expect(configStore.get('channel')).toBe('xiaohongshu');
    expect(configStore.get('accountId')).toBe('a2');
    expect(configStore.get('patrol.intervalMs')).toBe(7000);
    expect(configStore.isAccountEnabled('xiaohongshu', 'a2')).toBe(false);
  });

  it('加密文件 + 正确口令 → 解密后热更新', async () => {
    if (!hasSubtle()) return;
    const plain = { serverUrl: 'http://enc:8204', token: 'enc-token' };
    const payload = await encryptJSON(plain, 'my-pass');
    const r = await importConfig(JSON.stringify(payload), 'my-pass');
    expect(r.ok).toBe(true);
    expect(configStore.get('serverUrl')).toBe('http://enc:8204');
    expect(configStore.get('token')).toBe('enc-token');
  });

  it('加密文件 + 错误口令 → 导入失败且不更新', async () => {
    if (!hasSubtle()) return;
    const payload = await encryptJSON({ serverUrl: 'http://enc:8204' }, 'right');
    await expect(importConfig(JSON.stringify(payload), 'wrong')).rejects.toThrow('口令错误');
    expect(configStore.get('serverUrl')).not.toBe('http://enc:8204');
  });

  it('非法 JSON → 导入失败', async () => {
    await expect(importConfig('not json {{{', '')).rejects.toThrow('非法 JSON');
  });

  it('非对象 JSON → 导入失败', async () => {
    await expect(importConfig('"just a string"', '')).rejects.toThrow('不是对象');
  });

  it('缺 data 且非 encrypted → 结构无效', async () => {
    await expect(importConfig(JSON.stringify({ schema: 1, foo: 1 }), '')).rejects.toThrow('结构无效');
  });

  it('File 对象输入（含 .text()）也能解析', async () => {
    const file = { text: async () => JSON.stringify({ schema: 1, encrypted: false, data: { serverUrl: 'http://f:8204' } }) };
    const r = await importConfig(file, '');
    expect(r.ok).toBe(true);
    expect(configStore.get('serverUrl')).toBe('http://f:8204');
  });
});

describe('config-io downloadConfigJson', () => {
  it('触发浏览器下载（stub URL.createObjectURL / createElement）', () => {
    const origCreate = URL.createObjectURL;
    const origRevoke = URL.revokeObjectURL;
    const origAppend = document.body.appendChild;
    const origRemove = document.body.removeChild;
    const origCreateEl = document.createElement;
    URL.createObjectURL = vi.fn(() => 'blob:fake-url');
    URL.revokeObjectURL = vi.fn();
    document.body.appendChild = vi.fn();
    document.body.removeChild = vi.fn();
    const click = vi.fn();
    const anchor = { href: '', download: '', click };
    document.createElement = vi.fn(() => anchor);
    try {
      const r = downloadConfigJson('{"a":1}', 'cfg.json');
      expect(r.ok).toBe(true);
      expect(anchor.href).toBe('blob:fake-url');
      expect(anchor.download).toBe('cfg.json');
      expect(click).toHaveBeenCalled();
      expect(document.body.appendChild).toHaveBeenCalledWith(anchor);
      expect(document.body.removeChild).toHaveBeenCalledWith(anchor);
    } finally {
      URL.createObjectURL = origCreate;
      URL.revokeObjectURL = origRevoke;
      document.body.appendChild = origAppend;
      document.body.removeChild = origRemove;
      document.createElement = origCreateEl;
    }
  });

  it('环境不支持 → 返回 { ok:false, error }', () => {
    const origCreate = URL.createObjectURL;
    URL.createObjectURL = vi.fn(() => { throw new Error('unsupported'); });
    try {
      const r = downloadConfigJson('{}', 'x.json');
      expect(r.ok).toBe(false);
      expect(r.error).toContain('unsupported');
    } finally {
      URL.createObjectURL = origCreate;
    }
  });
});

describe('config-io getSubtle', () => {
  it('返回 globalThis.crypto.subtle（Node 22 存在）', () => {
    const s = getSubtle();
    if (hasSubtle()) expect(s).toBeTruthy();
    else expect(s).toBeNull();
  });
});

