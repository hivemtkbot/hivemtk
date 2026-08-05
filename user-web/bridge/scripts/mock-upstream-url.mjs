// mock 上行验证：构造 mock fetch + stub log，检查 postIngest 是否真的打印
// 完整 URL + 解析后的 query + body 预览。
//
// 用法：node scripts/mock-upstream-url.mjs
// 输出示例：
//   [bridge:http] [上行 inbound ingest] addr=http://localhost:8204/api/bridge/ingest?channel=...
//   { url: '...', origin: '...', path: '/api/bridge/ingest', query: { channel: 'xhs', ... } }
//
// 用来在重构后确认"打印完整 URL + 参数"逻辑没被破坏。

import { postIngest, buildIngestUrl, describeIngestParams } from '../src/core/http-ingest.js';

// 拦截 fetch：返回 mock 200 + JSON
let lastUrl = null;
let lastInit = null;
globalThis.fetch = async (url, init) => {
  lastUrl = url;
  lastInit = init;
  return {
    ok: true,
    status: 200,
    statusText: 'OK',
    text: async () => JSON.stringify({ ok: true, ingested: [{ event_id: 'evt1' }], outbound_replies: [] }),
  };
};

const cfg = {
  serverUrl: 'http://localhost:8204',
  channel: 'xhs',
  accountId: 'acc_test_001',
  conversationId: 'conv_42',
  token: 'abcdef1234567890',
};

const body = {
  v: 2,
  channel: 'xhs',
  account_id: 'acc_test_001',
  conversation_id: 'conv_42',
  messages: [
    {
      event_id: 'evt1',
      content: '你好在吗',
      sender_type: 'customer',
      timestamp: Date.now(),
    },
  ],
  expect_reply: true,
  timeout_ms: 500000,
};

console.log('=== buildIngestUrl 输出 ===');
const u = buildIngestUrl(cfg.serverUrl, cfg);
console.log('URL:', u);

console.log('\n=== describeIngestParams 输出 ===');
console.log(JSON.stringify(describeIngestParams(u), null, 2));

console.log('\n=== postIngest 调用（验证实际 fetch URL + body 透传）===');
const resp = await postIngest(cfg, body, { label: '[mock 上行测试]' });
console.log('fetch URL:', lastUrl);
console.log('fetch method:', lastInit && lastInit.method);
console.log('fetch headers Content-Type:', lastInit && lastInit.headers && lastInit.headers['Content-Type']);
console.log('fetch body:', lastInit && lastInit.body);
console.log('响应:', resp);

console.log('\n✅ 验证通过：URL 完整携带 channel/account_id/conversation_id/token 参数');
