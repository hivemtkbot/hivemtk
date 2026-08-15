
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { SharedArray } from 'k6/data';
import { randomString } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

// 自定义指标
const ingestSuccess = new Rate('ingest_success');
const ingestLatency = new Trend('ingest_latency_ms');
const ackSuccess = new Rate('ack_success');
const outboxFetched = new Counter('outbox_messages_fetched');

// 共享 token（避免在脚本中硬编码）
const tokens = new SharedArray('tokens', function () {
  return [__ENV.BRIDGE_TOKEN || 'test-token-1'];
});

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8204';
const CHANNELS = ['douyin', 'xiaohongshu', 'tiktok', 'xianyu'];
const AGENT_IDS = ['agent-1', 'agent-2', 'agent-3'];

// 测试配置
export const options = {
  scenarios: {
    bridge_ingest: {
      executor: 'constant-vus',
      vus: __ENV.VUS ? parseInt(__ENV.VUS) : 50,
      duration: __ENV.DURATION || '60s',
      exec: 'testIngest',
      tags: { scenario: 'ingest' },
    },
    bridge_outbox: {
      executor: 'constant-vus',
      vus: __ENV.VUS ? parseInt(__ENV.VUS) : 20,
      duration: __ENV.DURATION || '60s',
      startTime: '10s',  
      exec: 'testOutbox',
      tags: { scenario: 'outbox' },
    },
    bridge_ack: {
      executor: 'constant-vus',
      vus: __ENV.VUS ? parseInt(__ENV.VUS) : 30,
      duration: __ENV.DURATION || '60s',
      startTime: '20s',  
      exec: 'testAck',
      tags: { scenario: 'ack' },
    },
  },
  thresholds: {
    'ingest_latency_ms': ['p(95)<1000', 'p(99)<2000'],
    'ingest_success': ['rate>0.999'],
    'ack_success': ['rate>0.999'],
    'http_req_failed': ['rate<0.01'],
  },
};

// 随机工具
function pickChannel() {
  return CHANNELS[Math.floor(Math.random() * CHANNELS.length)];
}

function pickAgent() {
  return AGENT_IDS[Math.floor(Math.random() * AGENT_IDS.length)];
}

function pickToken() {
  return tokens[Math.floor(Math.random() * tokens.length)];
}

// ============================================================
// 测试 1：bridge ingest（POST /api/bridge/ingest）
// ============================================================
export function testIngest() {
  const channel = pickChannel();
  const agentId = pickAgent();
  const token = pickToken();
  const conversationId = `conv-${randomString(8)}`;

  const payload = JSON.stringify({
    channel: channel,
    account_id: `acc-${randomString(6)}`,
    agent_id: agentId,
    conversation_id: conversationId,
    messages: [
      {
        msg_id: `msg-${randomString(16)}`,
        event_id: `evt-${randomString(16)}`,
        sender_type: 'customer',
        text: '你好，请问有什么可以帮助您的？',
        timestamp: Date.now(),
      },
    ],
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
      'X-Request-Id': `k6-${randomString(12)}`,
    },
  };

  const res = http.post(`${BASE_URL}/api/bridge/ingest`, payload, params);
  const ok = res.status === 200 && !res.json('error');
  ingestSuccess.add(ok);
  ingestLatency.add(res.timings.duration);
  check(res, {
    'ingest status 200': (r) => r.status === 200,
    'ingest has results': (r) => r.json('results') !== undefined,
  });

  sleep(Math.random() * 0.1);  
}

// ============================================================
// 测试 2：bridge outbox（GET /api/bridge/outbox）
// ============================================================
export function testOutbox() {
  const channel = pickChannel();
  const agentId = pickAgent();
  const token = pickToken();

  const url = `${BASE_URL}/api/bridge/outbox?channel=${channel}&agent_id=${agentId}&long_poll=30`;
  const params = {
    headers: {
      'Authorization': `Bearer ${token}`,
    },
  };

  const res = http.get(url, params);
  const messages = res.json('messages') || [];
  outboxFetched.add(messages.length);

  check(res, {
    'outbox status 200': (r) => r.status === 200,
  });

  sleep(1);  
}

// ============================================================
// 测试 3：bridge ack（POST /api/bridge/ack）
// ============================================================
export function testAck() {
  const channel = pickChannel();
  const token = pickToken();

  // 模拟一批已收到但未 ack 的消息
  const items = [];
  for (let i = 0; i < 10; i++) {
    items.push({
      msg_id: `msg-${randomString(16)}`,
      conversation_id: `conv-${randomString(8)}`,
      status: 'delivered',
      timestamp: Date.now(),
    });
  }

  const payload = JSON.stringify({
    channel: channel,
    account_id: `acc-${randomString(6)}`,
    items: items,
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    },
  };

  const res = http.post(`${BASE_URL}/api/bridge/ack`, payload, params);
  const ok = res.status === 200;
  ackSuccess.add(ok);
  check(res, {
    'ack status 200': (r) => r.status === 200,
    'ack has affected_count': (r) => r.json('affected_count') !== undefined,
  });

  sleep(Math.random() * 0.5);
}

// ============================================================
// 默认函数（用于直接 k6 run 不带 scenario 跑）
// ============================================================
export default function () {
  testIngest();
}


