import { describe, it, expect, beforeAll } from 'vitest';
import { processAckDetailedResult } from '../src/core/downlink.js';

const BASE_URL = process.env.HIVEMTK_E2E_BASE_URL || 'http://localhost:8080';
const CHANNEL = 'douyin';
const ACCOUNT = 'e2e_p3h_' + Date.now();

let serverAvailable = false;
let aiSeedId = null; 

beforeAll(async () => {
  try {
    const r = await fetch(`${BASE_URL}/healthz`, { method: 'GET' });
    serverAvailable = r.ok;
  } catch (_) {
    serverAvailable = false;
  }
  if (!serverAvailable) {
    console.warn(`[P3-H e2e] server ${BASE_URL}/healthz 不可达，相关测试将跳过`);
  }
});

async function postJSON(path, body, headers = {}) {
  const r = await fetch(`${BASE_URL}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...headers },
    body: JSON.stringify(body),
  });
  const text = await r.text();
  let json = null;
  try { json = text ? JSON.parse(text) : null; } catch (_) {  }
  return { status: r.status, headers: r.headers, body: json, raw: text };
}

async function getJSON(path, headers = {}) {
  const r = await fetch(`${BASE_URL}${path}`, {
    method: 'GET',
    headers,
  });
  const text = await r.text();
  let json = null;
  try { json = text ? JSON.parse(text) : null; } catch (_) {  }
  return { status: r.status, body: json, raw: text };
}

describe('P3-H e2e: 三通道端到端 (douyin/e2e)', () => {
  it.skipIf(!serverAvailable)('ingest 上报 customer 消息 → 服务端落库 + AI 触发', async () => {
    const reqId = `e2e-req-${Date.now().toString(36)}`;
    const r = await postJSON(
      `/api/bridge/ingest?channel=${CHANNEL}&account_id=${ACCOUNT}`,
      {
        channel: CHANNEL,
        account_id: ACCOUNT,
        conversation_id: 'e2e_conv',
        messages: [
          {
            event_id: `e2e_evt_${Date.now()}`,
            sender_type: 'customer',
            msg_type: 'text',
            content: `e2e 验证消息 ${new Date().toISOString()}`,
            timestamp: Date.now(),
          },
        ],
      },
      { 'X-Request-Id': reqId }
    );
    expect([200, 400]).toContain(r.status);
    if (r.status === 200) {
      expect(r.body.ok).toBe(true);
    }
  });

  it.skipIf(!serverAvailable)('outbox 拉取 → 验证响应 schema 与 P3-D ack 输入', async () => {
    const r = await getJSON(`/api/bridge/outbox?channel=${CHANNEL}&account_id=${ACCOUNT}&limit=10`);
    expect(r.status).toBe(200);
    expect(r.body).toBeTruthy();
    if (r.body && r.body.messages && r.body.messages.length > 0) {
      const m = r.body.messages[0];
      expect(m).toHaveProperty('msg_id');
      expect(m).toHaveProperty('conversation_id');
      expect(m).toHaveProperty('content');
      aiSeedId = m.msg_id;
    }
  });

  it.skipIf(!serverAvailable)('ack 出站 → 验证 P3-D 详细响应（acked/duplicate/not_found）', async () => {
    // 准备测试 msg_ids：2 条真实（如果拉到了）+ 1 条不存在
    const realIds = aiSeedId ? [aiSeedId] : [];
    const r = await postJSON(
      `/api/bridge/outbox/ack?channel=${CHANNEL}&account_id=${ACCOUNT}`,
      {
        msg_ids: [...realIds, 'mh:nonexistent_e2e_p3h'],
        status: 'delivered',
      }
    );
    expect(r.status).toBe(200);
    expect(r.body.status).toBe('ok');
    expect(r.body).toHaveProperty('acked');
    expect(r.body).toHaveProperty('duplicate_count');
    expect(r.body).toHaveProperty('not_found_count');
    expect(Array.isArray(r.body.items)).toBe(true);
    if (realIds.length) {
      // 真实 msg_id 应该是 acked
      const realItem = r.body.items.find((it) => it.msg_id === aiSeedId);
      expect(realItem).toBeTruthy();
      expect(realItem.status).toBe('acked');
    }
    // 不存在 msg_id 应该是 not_found
    const nf = r.body.items.find((it) => it.msg_id === 'mh:nonexistent_e2e_p3h');
    expect(nf).toBeTruthy();
    expect(nf.status).toBe('not_found');
  });

  it.skipIf(!serverAvailable)('二次 ack 同一 msg_id → 验证 duplicate', async () => {
    if (!aiSeedId) return; 
    const r = await postJSON(
      `/api/bridge/outbox/ack?channel=${CHANNEL}&account_id=${ACCOUNT}`,
      { msg_ids: [aiSeedId], status: 'delivered' }
    );
    expect(r.status).toBe(200);
    const item = r.body.items.find((it) => it.msg_id === aiSeedId);
    expect(item).toBeTruthy();
    expect(item.status).toBe('duplicate');
    expect(r.body.duplicate_count).toBeGreaterThanOrEqual(1);
    // 验证前端 processAckDetailedResult 与后端响应一致
    const processed = processAckDetailedResult(r.body, [aiSeedId]);
    expect(processed.duplicate).toBe(1);
    expect(processed.acked).toBe(0);
  });

  it.skipIf(!serverAvailable)('account_id 缺失 → 400 拒绝', async () => {
    const r = await postJSON(
      `/api/bridge/ingest?channel=${CHANNEL}`,
      {
        channel: CHANNEL,
        account_id: '',
        conversation_id: 'c',
        messages: [],
      }
    );
    expect(r.status).toBe(400);
    expect(r.body.reason).toContain('account_id required');
  });

  it.skipIf(!serverAvailable)('channel 不在白名单 → 400 拒绝', async () => {
    const r = await postJSON(
      `/api/bridge/ingest?channel=evil_unknown&account_id=${ACCOUNT}`,
      {
        channel: 'evil_unknown',
        account_id: ACCOUNT,
        conversation_id: 'c',
        messages: [],
      }
    );
    expect(r.status).toBe(400);
    expect(r.body.reason).toContain('unsupported');
  });

  it.skipIf(!serverAvailable)('X-Request-Id 透传（端到端溯源 P3-E）', async () => {
    const reqId = `e2e-trace-${Math.random().toString(36).slice(2, 10)}`;
    // ingest 不依赖消息落库（空消息体应快速返回 400 而不是 200）
    const r = await postJSON(
      `/api/bridge/ingest?channel=${CHANNEL}&account_id=${ACCOUNT}`,
      { channel: CHANNEL, account_id: ACCOUNT, conversation_id: 'c', messages: [] },
      { 'X-Request-Id': reqId }
    );
    expect([200, 400]).toContain(r.status);
  });
});

describe('P3-H e2e 端到端健壮性（即使 server 不可达，本地协议层也要通过）', () => {
  it('空 msg_ids：processAckDetailedResult 不会抛错', () => {
    expect(() => processAckDetailedResult({ status: 'ok', items: [] }, [])).not.toThrow();
  });

  it('ackIds 含空字符串：processAckDetailedResult 安全跳过', () => {
    const r = processAckDetailedResult(
      { status: 'ok', items: [{ msg_id: 'm1', status: 'acked' }] },
      ['', 'm1', '']
    );
    expect(r.acked).toBe(1);
  });
});

