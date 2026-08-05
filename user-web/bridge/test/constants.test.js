// bridge 默认值与 DEFAULTS.md 一致性测试
//
// 目的：
//   1) 验证 constants.js 的所有值都有意义（非空、合法）
//   2) 验证前端/后端数值对齐（防止 client/server 数字漂移）
//   3) 防止后续改动时遗漏文档源引用
//
// 参考模式：user-server/internal/pkg/utils/config/inference_load_test.go

import { describe, it, expect } from 'vitest';
import {
  DEFAULT_USER_SERVER,
  PLATFORM_ENTRY_URLS,
  RATE_LIMIT_DEFAULTS,
  WS_CLIENT_DEFAULTS,
  UI_DEFAULTS,
  PROTOCOL,
  SECURITY,
} from '../src/core/constants.js';

describe('DEFAULT_USER_SERVER', () => {
  it('host 必须是 localhost（dev 默认）', () => {
    expect(DEFAULT_USER_SERVER.host).toBe('localhost');
  });

  it('port 必须为 8204（与 DEVELOPMENT.md 端口表 / Dockerfile ENV 一致）', () => {
    expect(DEFAULT_USER_SERVER.port).toBe(8204);
  });

  it('baseUrl 必须由 host+port 正确拼接', () => {
    expect(DEFAULT_USER_SERVER.baseUrl).toBe(`http://${DEFAULT_USER_SERVER.host}:${DEFAULT_USER_SERVER.port}`);
  });

  it('healthPaths 必须按优先级排序：/health 优先', () => {
    expect(DEFAULT_USER_SERVER.healthPaths[0]).toBe('/health');
    expect(DEFAULT_USER_SERVER.healthPaths).toContain('/healthz');
    expect(DEFAULT_USER_SERVER.healthPaths).toContain('/readyz');
  });

  it('wsPath 必须为 /api/ws/bridge（与 router/service_routes.go:90 一致）', () => {
    expect(DEFAULT_USER_SERVER.wsPath).toBe('/api/ws/bridge');
  });

  it('profile 必须为 dev', () => {
    expect(DEFAULT_USER_SERVER.profile).toBe('dev');
  });
});

describe('PLATFORM_ENTRY_URLS', () => {
  it('抖音/小红书/TikTok 三个 URL 都必须 https://', () => {
    for (const [k, url] of Object.entries(PLATFORM_ENTRY_URLS)) {
      expect(url.startsWith('https://'), `${k} 必须 https`).toBe(true);
    }
  });

  it('三个渠道名必须与 PROTOCOL.CHANNELS 严格一致', () => {
    for (const k of Object.keys(PLATFORM_ENTRY_URLS)) {
      expect(Object.values(PROTOCOL.CHANNELS)).toContain(k);
    }
  });
});

describe('RATE_LIMIT_DEFAULTS', () => {
  it('所有字段必须为正数', () => {
    for (const [k, v] of Object.entries(RATE_LIMIT_DEFAULTS)) {
      expect(typeof v, `${k} 必须为 number`).toBe('number');
      expect(v, `${k} 必须 > 0`).toBeGreaterThan(0);
    }
  });

  it('jitter 区间必须合法（min < max）', () => {
    expect(RATE_LIMIT_DEFAULTS.jitterMinMs).toBeLessThan(RATE_LIMIT_DEFAULTS.jitterMaxMs);
  });

  it('accountCapacity 必须 ≤ 后端兜底 60/min（前端更严格）', () => {
    // 后端 hub.go DeliverRateLimitPerMin = 60；前端必须 <= 60
    expect(RATE_LIMIT_DEFAULTS.accountCapacity).toBeLessThanOrEqual(60);
  });

  it('minIntervalMs 必须 < jitterMaxMs（避免出现负 waitHintMs）', () => {
    expect(RATE_LIMIT_DEFAULTS.minIntervalMs).toBeLessThan(RATE_LIMIT_DEFAULTS.jitterMaxMs);
  });

  it('冻结对象，防止就地修改', () => {
    expect(Object.isFrozen(RATE_LIMIT_DEFAULTS)).toBe(true);
  });
});

describe('WS_CLIENT_DEFAULTS', () => {
  it('serverIdleTimeoutMs (25s) 必须 < 服务端 pongWait (60s)', () => {
    expect(WS_CLIENT_DEFAULTS.serverIdleTimeoutMs).toBeLessThan(60 * 1000);
    expect(WS_CLIENT_DEFAULTS.serverIdleTimeoutMs).toBeGreaterThan(0);
  });

  it('reconnect 指数退避必须合法', () => {
    expect(WS_CLIENT_DEFAULTS.reconnectBaseMs).toBeLessThanOrEqual(WS_CLIENT_DEFAULTS.reconnectMaxMs);
    expect(WS_CLIENT_DEFAULTS.reconnectJitterMs).toBeLessThan(WS_CLIENT_DEFAULTS.reconnectBaseMs);
  });

  it('冻结对象', () => {
    expect(Object.isFrozen(WS_CLIENT_DEFAULTS)).toBe(true);
  });
});

describe('UI_DEFAULTS', () => {
  it('healthCheckTimeoutMs 必须 1-10s（不能太短也不能太久）', () => {
    expect(UI_DEFAULTS.healthCheckTimeoutMs).toBeGreaterThanOrEqual(1000);
    expect(UI_DEFAULTS.healthCheckTimeoutMs).toBeLessThanOrEqual(10_000);
  });
});

describe('PROTOCOL', () => {
  it('CHANNELS 值：2026-08-05 渠道编码统一后全部为全名（无 _web 后缀）', () => {
    // 2026-08-05 渠道编码统一：bridge 渠道名 = 平台全名（xiaohongshu/douyin/kuaishou/xianyu/tiktok），
    // 与后端 model.Channel*、SQL 数据、channel_agent_bindings 完全一致。
    const expected = new Set(['xiaohongshu', 'douyin', 'kuaishou', 'xianyu', 'tiktok']);
    for (const v of Object.values(PROTOCOL.CHANNELS)) {
      expect(expected.has(v)).toBe(true);
    }
  });

  it('FRAME 名称必须与 user-server/internal/bridge/frames.go 严格一致', () => {
    // 协议即契约，禁改字面量（DEVELOPMENT.md 强约束）
    expect(PROTOCOL.FRAME).toEqual({
      REGISTER: 'register',
      INBOUND: 'inbound_message',
      HISTORY: 'history',
      OUTBOUND: 'outbound_reply',
      PONG: 'pong',
      PING: 'ping',
      ACK: 'ack',
      ERROR: 'error',
    });
  });

  it('冻结对象', () => {
    expect(Object.isFrozen(PROTOCOL)).toBe(true);
  });
});

describe('SECURITY', () => {
  it('maxReplyContentBytes 必须与服务端 handler.go maxReplyContentBytes 一致', () => {
    // 服务端：4 * 1024 = 4096
    // 前端必须严格相等（避免服务端截断后用户看到残缺）
    expect(SECURITY.maxReplyContentBytes).toBe(4 * 1024);
  });

  it('logMaskMaxChars 必须为正', () => {
    expect(SECURITY.logMaskMaxChars).toBeGreaterThan(0);
    expect(SECURITY.logMaskMaxChars).toBeLessThanOrEqual(100);
  });
});

describe('DEFAULTS 文档源完整性', () => {
  it('每个顶层常量都必须在 DEFAULTS.md 出现（人工检查）', () => {
    // 此项测试为占位提醒：每次新增顶层常量请同步 DEFAULTS.md
    const expected = [
      'DEFAULT_USER_SERVER',
      'PLATFORM_ENTRY_URLS',
      'RATE_LIMIT_DEFAULTS',
      'WS_CLIENT_DEFAULTS',
      'UI_DEFAULTS',
      'PROTOCOL',
      'SECURITY',
    ];
    // 运行时检查：每个常量都已 import
    expect(typeof DEFAULT_USER_SERVER).toBe('object');
    expect(typeof PLATFORM_ENTRY_URLS).toBe('object');
    expect(typeof RATE_LIMIT_DEFAULTS).toBe('object');
    expect(typeof WS_CLIENT_DEFAULTS).toBe('object');
    expect(typeof UI_DEFAULTS).toBe('object');
    expect(typeof PROTOCOL).toBe('object');
    expect(typeof SECURITY).toBe('object');
    expect(expected.length).toBe(7);
  });
});
