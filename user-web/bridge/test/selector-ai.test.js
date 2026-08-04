// selector-ai 单元测试：验证「云端 LLM 动态选择器」核心链路——
// 脱敏快照序列化、缓存读写、AI 优先于规则兜底、刷新回写缓存。
// 这是把「硬编码选择器」解耦为「云端生成产物」的关键保障。
import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
  snapshotDom,
  mergeSelectors,
  getCachedSpec,
  saveCache,
  refreshSpec,
  _resetMemoryCache,
  compileExtractor,
  runExtractor,
  validateSpec,
} from '../src/core/selector-ai.js';

const CHANNEL = 'douyin_web';
const DOMAIN = 'im.douyin.com';

beforeEach(() => {
  localStorage.clear();
  _resetMemoryCache();
  vi.restoreAllMocks();
});

// 1) 快照序列化：剔除文本/图片，保留结构，剪枝纯噪声
describe('snapshotDom', () => {
  it('剔除文本与图片地址，仅保留 tag/class/关键属性', () => {
    const root = document.createElement('div');
    root.className = 'im-list';
    root.setAttribute('data-e2e', 'msg-list');
    const item = document.createElement('div');
    item.className = 'msg-item';
    item.setAttribute('role', 'listitem');
    item.textContent = '这是私密聊天内容不应出现在快照里'; // 应被丢弃
    const img = document.createElement('img');
    img.src = 'https://secret.cdn/x.png'; // 应被丢弃
    item.appendChild(img);
    root.appendChild(item);

    const snap = JSON.parse(snapshotDom(root));
    expect(snap.t).toBe('div');
    expect(snap.c).toContain('im-list');
    expect(snap.a['data-e2e']).toBe('msg-list');
    const child = snap.ch[0];
    expect(child.c).toContain('msg-item');
    expect(child.a.role).toBe('listitem');
    // 绝对不能包含聊天文本或图片地址
    expect(JSON.stringify(snap)).not.toContain('私密聊天');
    expect(JSON.stringify(snap)).not.toContain('secret.cdn');
  });

  it('剪枝：无 class/属性且无后代的纯噪声节点被丢弃', () => {
    const root = document.createElement('div');
    root.className = 'keep';
    const noise = document.createElement('span'); // 无 class、无属性
    const noiseChild = document.createElement('i'); // 也无 class
    noise.appendChild(noiseChild);
    root.appendChild(noise);

    const snap = JSON.parse(snapshotDom(root));
    // root 保留，但 noise 节点（及其子）因无任何结构信息被剪掉
    expect(snap.c).toContain('keep');
    expect(snap.ch).toBeUndefined();
  });
});

// 2) 缓存读写
describe('cache', () => {
  it('saveCache → getCachedSpec 往返一致', () => {
    const spec = {
      version: 2,
      message_item: ['.ai-bubble'],
      message_list: ['.ai-list'],
    };
    saveCache(CHANNEL, DOMAIN, spec);
    const got = getCachedSpec(CHANNEL, DOMAIN);
    expect(got.message_item).toEqual(['.ai-bubble']);
    expect(got.message_list).toEqual(['.ai-list']);
  });

  it('版本不匹配 → 缓存视为失效', () => {
    saveCache(CHANNEL, DOMAIN, { version: 1, message_item: ['x'] });
    // 模拟 SPEC_VERSION 升级后旧缓存失效（实现依赖内部版本号，这里直接写错版本）
    localStorage.setItem(
      'hivebridge:sel:douyin_web:im.douyin.com',
      JSON.stringify({ version: 999, ts: Date.now(), spec: { message_item: ['old'] } })
    );
    expect(getCachedSpec(CHANNEL, DOMAIN)).toBeNull();
  });
});

// 3) mergeSelectors：有缓存时 AI 优先（放前），规则兜底（放后）
describe('mergeSelectors', () => {
  const FB = {
    itemSelectors: ['[data-e2e="msg-item-content"]'],
    listSelectors: ['.im-message-list'],
    textSelectors: ['.text'],
    inputSelectors: ['div[contenteditable]'],
    sendSelectors: ['[class*="send"]'],
    selfMarkers: ['.self'],
    otherMarkers: ['.other'],
  };

  it('无缓存 → 仅返回规则兜底', () => {
    const m = mergeSelectors(CHANNEL, DOMAIN, FB);
    expect(m.itemSelectors).toEqual(['[data-e2e="msg-item-content"]']);
    expect(m.listSelectors).toEqual(['.im-message-list']);
  });

  it('有缓存 → AI 选择器排在规则之前（AI 优先）', () => {
    saveCache(CHANNEL, DOMAIN, {
      version: 2,
      message_item: ['.ai-item'],
      message_list: ['.ai-list'],
      text: ['.ai-text'],
      input_box: ['.ai-input'],
      send_button: ['.ai-send'],
      self_marker: ['.ai-self'],
      other_marker: ['.ai-other'],
    });
    const m = mergeSelectors(CHANNEL, DOMAIN, FB);
    // AI 在前
    expect(m.itemSelectors[0]).toBe('.ai-item');
    expect(m.itemSelectors).toContain('[data-e2e="msg-item-content"]');
    expect(m.listSelectors[0]).toBe('.ai-list');
    expect(m.textSelectors[0]).toBe('.ai-text');
    expect(m.inputSelectors[0]).toBe('.ai-input');
    expect(m.sendSelectors[0]).toBe('.ai-send');
    expect(m.selfMarkers[0]).toBe('.ai-self');
    expect(m.otherMarkers[0]).toBe('.ai-other');
  });

  it('不修改传入的规则兜底对象', () => {
    const before = FB.itemSelectors.slice();
    mergeSelectors(CHANNEL, DOMAIN, FB);
    expect(FB.itemSelectors).toEqual(before);
  });
});

// 4) refreshSpec：调云端成功 → 写缓存；失败 → 返回 null（回退规则）
describe('refreshSpec', () => {
  it('云端返回 spec → 写入缓存并返回', async () => {
    const fakeSpec = {
      version: 2,
      channel: CHANNEL,
      domain: DOMAIN,
      message_item: ['.cloud-item'],
      message_list: ['.cloud-list'],
    };
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: true,
        json: async () => ({ enabled: true, source: 'llm', spec: fakeSpec }),
      }))
    );
    const result = await refreshSpec(CHANNEL, DOMAIN);
    expect(result.message_item).toEqual(['.cloud-item']);
    expect(getCachedSpec(CHANNEL, DOMAIN).message_item).toEqual(['.cloud-item']);
  });

  it('云端失败 → 返回 null，不写缓存', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ ok: false, status: 500, json: async () => ({}) }))
    );
    const result = await refreshSpec(CHANNEL, DOMAIN);
    expect(result).toBeNull();
    expect(getCachedSpec(CHANNEL, DOMAIN)).toBeNull();
  });

  it('页面有聊天容器但选择器命中 0 → 拒收，不写缓存（避免假阳性）', async () => {
    const chat = document.createElement('div');
    chat.className = 'im-message-list';
    chat.innerHTML = '<div class="msg-item">hi</div>';
    document.body.appendChild(chat);
    try {
      vi.stubGlobal(
        'fetch',
        vi.fn(async () => ({
          ok: true,
          json: async () => ({
            enabled: true,
            source: 'llm',
            spec: { version: 1, message_item: ['.definitely-not-exist-xyz'], message_list: [] },
          }),
        }))
      );
      const result = await refreshSpec(CHANNEL, DOMAIN);
      expect(result).toBeNull();
      expect(getCachedSpec(CHANNEL, DOMAIN)).toBeNull();
    } finally {
      document.body.removeChild(chat);
    }
  });

  it('选择器在真实 DOM 命中 → 放行并写缓存', async () => {
    const chat = document.createElement('div');
    chat.className = 'im-message-list';
    const item = document.createElement('div');
    item.className = 'msg-item';
    chat.appendChild(item);
    document.body.appendChild(chat);
    try {
      vi.stubGlobal(
        'fetch',
        vi.fn(async () => ({
          ok: true,
          json: async () => ({
            enabled: true,
            source: 'llm',
            spec: { version: 1, message_item: ['.msg-item'], message_list: ['.im-message-list'] },
          }),
        }))
      );
      const result = await refreshSpec(CHANNEL, DOMAIN);
      expect(result).not.toBeNull();
      expect(result.message_item).toEqual(['.msg-item']);
      expect(getCachedSpec(CHANNEL, DOMAIN).message_item).toEqual(['.msg-item']);
    } finally {
      document.body.removeChild(chat);
    }
  });
});

// 5) extractor（可执行 JS 抽取器）：LLM 返回 JS，经 new Function 沙箱执行，彻底不依赖固定选择器 schema
describe('extractor', () => {
  it('compileExtractor 拒绝含 fetch 等危险写法的源码', () => {
    const r = compileExtractor('function(d,w){ fetch("http://evil"); return {messages:[]}; }');
    expect(r.ok).toBe(false);
    expect(r.error).toMatch(/forbidden/i);
  });

  it('compileExtractor 接受合法函数体并可执行', () => {
    const r = compileExtractor('function(d,w){ return { messages: [{text:"hi", self:false}] }; }');
    expect(r.ok).toBe(true);
    expect(typeof r.fn).toBe('function');
  });

  it('runExtractor 在真实 DOM 执行抽取器，归一化为内部消息（customer/self）', () => {
    const root = document.createElement('div');
    const a = document.createElement('div'); a.className = 'msg-item'; a.textContent = '你好';
    const b = document.createElement('div'); b.className = 'msg-item self'; b.textContent = '在的';
    root.appendChild(a); root.appendChild(b);
    document.body.appendChild(root);
    try {
      saveCache(CHANNEL, DOMAIN, {
        version: 2,
        extractor:
          'function(doc, win){' +
          '  var items = doc.querySelectorAll(".msg-item");' +
          '  var messages = [];' +
          '  items.forEach(function(el){ messages.push({ text: el.textContent, self: el.classList.contains("self") }); });' +
          '  return { messages: messages, input_box: ".inp", send_button: ".send" };' +
          '}',
      });
      const r = runExtractor(CHANNEL, DOMAIN);
      expect(r).not.toBeNull();
      expect(r.messages.length).toBe(2);
      expect(r.messages[0].text).toBe('你好');
      expect(r.messages[0].sender_type).toBe('customer');
      expect(r.messages[1].text).toBe('在的');
      expect(r.messages[1].sender_type).toBe('self');
      expect(r.input_box).toBe('.inp');
      expect(r.send_button).toBe('.send');
    } finally {
      document.body.removeChild(root);
    }
  });

  it('runExtractor 无缓存时返回 null（回退选择器路径）', () => {
    expect(runExtractor(CHANNEL, DOMAIN)).toBeNull();
  });

  it('validateSpec：含可编译 extractor 的 spec 通过校验', () => {
    expect(validateSpec({ extractor: 'function(d,w){ return { messages: [] }; }' })).toBe(true);
  });

  it('validateSpec：extractor 危险且无选择器兜底 → 不通过（拒收）', () => {
    expect(validateSpec({ extractor: 'function(d,w){ fetch("x"); return {messages:[]}; }' })).toBe(false);
  });

  it('refreshSpec 写入含 extractor 的 spec 后，runExtractor 可执行', async () => {
    const root = document.createElement('div');
    const item = document.createElement('div'); item.className = 'msg-item'; item.textContent = '云抽取';
    root.appendChild(item);
    document.body.appendChild(root);
    try {
      vi.stubGlobal(
        'fetch',
        vi.fn(async () => ({
          ok: true,
          json: async () => ({
            enabled: true,
            source: 'llm',
            spec: {
              version: 2,
              extractor:
                'function(doc,win){ var els=doc.querySelectorAll(".msg-item"); var m=[]; ' +
                'els.forEach(function(e){ m.push({text:e.textContent, self:false}); }); return {messages:m}; }',
            },
          }),
        }))
      );
      const result = await refreshSpec(CHANNEL, DOMAIN);
      expect(result).not.toBeNull();
      expect(typeof result.extractor).toBe('string');
      const r = runExtractor(CHANNEL, DOMAIN);
      expect(r.messages[0].text).toBe('云抽取');
    } finally {
      document.body.removeChild(root);
    }
  });
});
