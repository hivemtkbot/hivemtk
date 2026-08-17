import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { PollingLoop } from '../src/core/polling-loop.js';
import { BRIDGE_THREE_CHANNEL } from '../src/core/constants.js';

// PollingLoop 现在只负责 downlink 下发轮询；巡检由 ChannelAdapter.startPatrol 自循环负责。

const getMeta = () => ({ accountId: 'acc-1' });
const getConfig = async () => ({ serverUrl: 'http://localhost:8204', token: 'tkn-1' });

describe('polling-loop / 生命周期', () => {
  it('start() 设置 downlink 定时器，stop() 清除', () => {
    const loop = new PollingLoop({ channels: ['douyin'], getAdapter: () => null, getConfig, getMeta });
    expect(loop._downlinkTimer).toBeNull();
    loop.start();
    expect(loop._downlinkTimer).not.toBeNull();
    expect(loop._patrolTimer).toBeUndefined();
    loop.stop();
    expect(loop._downlinkTimer).toBeNull();
  });

  it('start() 幂等：重复调用不创建多个 timer', () => {
    const loop = new PollingLoop({ channels: ['douyin'], getAdapter: () => null, getConfig, getMeta });
    loop.start();
    const t1 = loop._downlinkTimer;
    loop.start();
    expect(loop._downlinkTimer).toBe(t1);
    loop.stop();
  });

  it('stop() 后 _running 为 false', () => {
    const loop = new PollingLoop({ channels: ['douyin'], getAdapter: () => null, getConfig, getMeta });
    loop.start();
    loop.stop();
    expect(loop._running).toBe(false);
  });
});

describe('polling-loop / 下发轮询配置', () => {
  it('outboxPollIntervalMs=1500（下发轮询独立）', () => {
    expect(BRIDGE_THREE_CHANNEL.outboxPollIntervalMs).toBe(1500);
  });
});

describe('polling-loop / _downlinkSafe 防护', () => {
  it('stop() 后 _downlinkSafe 立即返回', async () => {
    const loop = new PollingLoop({ channels: ['douyin'], getAdapter: () => null, getConfig, getMeta });
    loop.start();
    loop.stop();
    await loop._downlinkSafe();
    expect(loop._downlinkInFlight).toBe(false);
  });

  it('无 config 时 _downlink 静默返回', async () => {
    const loop = new PollingLoop({ channels: ['douyin'], getAdapter: () => null, getConfig: () => null, getMeta });
    await loop._downlink();
  });
});
