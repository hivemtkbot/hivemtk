import { describe, it, expect, beforeEach, vi } from 'vitest';
import { configStore, buildDefaultConfig, STORAGE_KEY } from '../src/core/config-store.js';

describe('configStore / 默认值与合并 (P3-F 2026-08-15)', () => {
  beforeEach(() => {
    configStore.__reset();
  });

  it('buildDefaultConfig 包含 patrol/rate/circuit/version', () => {
    const def = buildDefaultConfig();
    expect(def.patrol.intervalMs).toBeGreaterThan(0);
    expect(def.rate.accountCapacity).toBeGreaterThan(0);
    expect(def.circuit.failureThreshold).toBe(5);
    expect(def.version).toBe(1);
  });

  it('_mergeWithDefault: 部分字段补全', () => {
    const merged = configStore._mergeWithDefault({ token: 'tkn' });
    expect(merged.token).toBe('tkn');
    expect(merged.patrol.intervalMs).toBeGreaterThan(0); 
    expect(merged.rate.accountCapacity).toBeGreaterThan(0);
  });

  it('_mergeWithDefault: 子对象深度合并', () => {
    const merged = configStore._mergeWithDefault({ patrol: { intervalMs: 99999 } });
    expect(merged.patrol.intervalMs).toBe(99999);
    expect(merged.patrol.switchMinMs).toBeGreaterThan(0); 
  });

  it('get 支持点号路径', () => {
    expect(configStore.get('patrol.intervalMs')).toBeGreaterThan(0);
    expect(configStore.get('rate.accountCapacity')).toBeGreaterThan(0);
    expect(configStore.get('nope.missing', 42)).toBe(42);
  });
});

describe('configStore / set + 事件 (P3-F)', () => {
  beforeEach(() => {
    configStore.__reset();
  });

  it('set 触发 change 事件（同步 on listener）', async () => {
    const events = [];
    configStore.on('change', (newCfg, oldCfg) => {
      events.push({ newCfg, oldCfg });
    });
    await configStore.set({ token: 'tkn-1' });
    expect(events.length).toBe(1);
    expect(events[0].newCfg.token).toBe('tkn-1');
    expect(events[0].oldCfg.token).not.toBe('tkn-1');
  });

  it('set 递增 version + updatedAt', async () => {
    const v0 = configStore.get('version');
    const t0 = configStore.get('updatedAt');
    await new Promise((r) => setTimeout(r, 5));
    await configStore.set({ token: 'a' });
    expect(configStore.get('version')).toBe(v0 + 1);
    expect(configStore.get('updatedAt')).toBeGreaterThan(t0);
  });

  it('set 持久化到 chrome.storage.local', async () => {
    const setSpy = vi.fn().mockResolvedValue();
    globalThis.chrome = {
      storage: { local: { get: vi.fn().mockResolvedValue({}), set: setSpy } },
    };
    await configStore.set({ token: 'persist-me' });
    expect(setSpy).toHaveBeenCalled();
    const callArg = setSpy.mock.calls[0][0];
    expect(callArg[STORAGE_KEY].token).toBe('persist-me');
  });
});


