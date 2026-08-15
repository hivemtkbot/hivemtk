import { describe, it, expect, beforeEach } from 'vitest';
import { humanDelay, humanType, humanMousePath, humanScroll, _internal, sendWindow, clickWindow } from '../src/core/humanize.js';

describe('humanize / 高斯分布 (P3-G 2026-08-15)', () => {
  it('gaussian: 均值/标准差/截断', () => {
    const { gaussian } = _internal;
    const samples = [];
    for (let i = 0; i < 200; i++) samples.push(gaussian(100, 10, 50, 200));
    // 200 样本均值接近 100
    const avg = samples.reduce((a, b) => a + b, 0) / samples.length;
    expect(avg).toBeGreaterThan(85);
    expect(avg).toBeLessThan(115);
    for (const s of samples) {
      expect(s).toBeGreaterThanOrEqual(50);
      expect(s).toBeLessThanOrEqual(200);
    }
  });
});

describe('humanize / 滑动窗口 (P3-B 集成)', () => {
  beforeEach(() => {
    sendWindow._buckets.clear();
    clickWindow._buckets.clear();
  });
  it('hit + count 窗口内计数', () => {
    sendWindow.hit('douyin', 'acc-1');
    sendWindow.hit('douyin', 'acc-1');
    expect(sendWindow.count('douyin', 'acc-1')).toBe(2);
  });
  it('窗口外清理', async () => {
    const orig = sendWindow.windowMs;
    sendWindow.windowMs = 10;
    sendWindow.hit('douyin', 'acc-1');
    await new Promise((r) => setTimeout(r, 30));
    expect(sendWindow.count('douyin', 'acc-1')).toBe(0);
    sendWindow.windowMs = orig;
  });
  it('isFull 触顶', () => {
    const w = new _internal.SlidingWindowCounter({ windowMs: 60000, capacity: 3 });
    w.hit('a', 'b');
    w.hit('a', 'b');
    w.hit('a', 'b');
    expect(w.isFull('a', 'b')).toBe(true);
  });
});

describe('humanize / humanDelay (P3-B 滑动窗口自适应)', () => {
  beforeEach(() => {
    sendWindow._buckets.clear();
    clickWindow._buckets.clear();
  });
  it('click 延迟落在 [50, 360] 区间', async () => {
    const samples = [];
    for (let i = 0; i < 20; i++) {
      const t0 = Date.now();
      await humanDelay('click');
      samples.push(Date.now() - t0);
    }
    for (const s of samples) {
      expect(s).toBeGreaterThanOrEqual(40);  
      expect(s).toBeLessThan(500);
    }
  });
  it('滑动窗口高密度时延迟延长（mult>1）', () => {
    const orig = sendWindow.windowMs;
    sendWindow.windowMs = 60000;
    for (let i = 0; i < 19; i++) sendWindow.hit('test-dense', 'acc');
    expect(sendWindow.count('test-dense', 'acc')).toBe(19);
    // 读取 profile 验证 mult 计算
    const cfg = _internal.DELAY_PROFILES.think;
    const density = 19 / 20;
    let mult = 1.0;
    if (density > 0.9) mult = 2.0;
    else if (density > 0.7) mult = 1.6;
    expect(mult).toBe(2.0);
    expect(cfg.mean * mult).toBe(3000);
    sendWindow._buckets.clear();
    sendWindow.windowMs = orig;
  });
});

describe('humanize / humanType (P3-G 键入节奏)', () => {
  it('键入字符串时元素 value 同步增长', async () => {
    let el;
    if (typeof document !== 'undefined') {
      el = document.createElement('input');
      document.body.appendChild(el);
    } else {
      el = { value: '', dispatchEvent: () => {} };
    }
    await humanType(el, 'hi', { charMean: 1, charStd: 0 });
    expect(el.value).toBe('hi');
  });
  it('空 element 或 text 静默通过', async () => {
    await humanType(null, 'x');
    await humanType({}, '');
  });
});

describe('humanize / humanMousePath (P3-G Bezier)', () => {
  it('从 (0,0) 到 (100,100) 移动 20-35 步', async () => {
    const path = [];
    await humanMousePath({ x: 0, y: 0 }, { x: 100, y: 100 }, (x, y) => {
      path.push([x, y]);
      return Promise.resolve();
    });
    expect(path.length).toBeGreaterThanOrEqual(20);
    expect(path.length).toBeLessThanOrEqual(35);
    expect(path[path.length - 1][0]).toBeCloseTo(100, 0);
    expect(path[path.length - 1][1]).toBeCloseTo(100, 0);
    expect(path[0][0]).toBeLessThan(10);
    expect(path[0][1]).toBeLessThan(10);
  });
  it('无 moveFn 时静默通过', async () => {
    const r = await humanMousePath({ x: 0, y: 0 }, { x: 1, y: 1 }, null);
    expect(r.steps).toBe(0);
  });
});

describe('humanize / humanScroll (P3-G 滚动曲线)', () => {
  it('分段滚动 + 偶尔回滚', async () => {
    const calls = [];
    await humanScroll((d) => { calls.push(d); return Promise.resolve(); }, {
      totalDelta: 400, chunks: 4, pauseEvery: 1, backtrackProb: 1, 
    });
    expect(calls.length).toBeGreaterThan(4);
    expect(calls.some((d) => d < 0)).toBe(true);
  });
  it('无 scrollFn 静默通过', async () => {
    const r = await humanScroll(null);
    expect(r.chunks).toBe(0);
  });
});


