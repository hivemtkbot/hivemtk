// 抖音私信同步回归测试：验证「多候选选择器 + 结构启发式」能在旧版与改版 DOM 下
// 都抓全消息；并验证旧版单一写死选择器在改版 DOM 下会大量漏抓（复现「几十个只上行 2 个」）。
// 环境：vite.config.js 已设 environment: 'jsdom'，全局 document / window / location 可用。
import { describe, it, expect, beforeEach } from 'vitest';
import { locateMessages, locateMessagesWithDiagnose, SelectorEngine } from '../src/core/selector-engine.js';
import { SEL } from '../src/channels/douyin.js';

const ITEM_SELS = SEL.MSG_ITEM.split(',').map((s) => s.trim());
const LIST_SELS = SEL.MSG_LIST.split(',').map((s) => s.trim()).filter(Boolean);

// 旧版抖音 DOM：气泡带 data-e2e="msg-item-content"
const OLD_DOM = `
<div class="im-message-list">
  <div class="msg-item" data-e2e="msg-item-content"><span class="text">你好，在吗</span></div>
  <div class="msg-item" data-e2e="msg-item-content"><span class="text">图片看一下</span></div>
  <div class="msg-item" data-e2e="msg-item-content"><span class="text">这件多少钱</span></div>
  <div class="msg-item" data-e2e="msg-item-content"><span class="text">可以优惠吗</span></div>
  <div class="msg-item" data-e2e="msg-item-content"><span class="text">什么时候发货</span></div>
  <div class="msg-item" data-e2e="msg-item-content"><img src="x.png"></div>
</div>`;

// 改版抖音 DOM：气泡没有 data-e2e，class 名完全变化（复现选择器失效）
const NEW_DOM = `
<section class="chat-thread-wrapper">
  <div class="bubble-row"><div class="bubble-body"><p>你好，在吗</p></div></div>
  <div class="bubble-row"><div class="bubble-body"><p>图片看一下</p></div></div>
  <div class="bubble-row"><div class="bubble-body"><p>这件多少钱</p></div></div>
  <div class="bubble-row"><div class="bubble-body"><p>可以优惠吗</p></div></div>
  <div class="bubble-row"><div class="bubble-body"><p>什么时候发货</p></div></div>
  <div class="bubble-row"><div class="bubble-body"><p>已拍下</p></div></div>
  <div class="bubble-row"><div class="bubble-body"><img src="y.png"></div></div>
  <div class="bubble-row"><div class="bubble-body"><span>撤回了一条消息</span></div></div>
</section>`;

beforeEach(() => {
  document.body.innerHTML = '';
});

describe('SelectorEngine 多候选 + 启发式定位', () => {
  it('旧版 DOM：抓全 6 条消息', () => {
    document.body.innerHTML = OLD_DOM;
    const { items } = locateMessages({ root: document, itemSelectors: ITEM_SELS, listSelectors: LIST_SELS });
    expect(items.length).toBe(6);
  });

  it('改版 DOM：启发式仍能抓全 8 条（不依赖 data-e2e）', () => {
    document.body.innerHTML = NEW_DOM;
    const { items } = locateMessages({ root: document, itemSelectors: ITEM_SELS, listSelectors: LIST_SELS });
    expect(items.length).toBe(8);
  });

  it('对比：旧单一选择器在改版 DOM 上完全失效（复现漏抓根因）', () => {
    document.body.innerHTML = NEW_DOM;
    const legacy = document.querySelectorAll('[data-e2e="msg-item-content"]');
    expect(legacy.length).toBe(0); // 改版后旧选择器命中 0 → 旧逻辑只抓 2 条（甚至 0）
  });

  it('非文字消息（图片/撤回）被保留，不被结构过滤丢弃', () => {
    document.body.innerHTML = NEW_DOM;
    const { items } = locateMessages({ root: document, itemSelectors: ITEM_SELS, listSelectors: LIST_SELS });
    const hasImg = items.some((el) => el.querySelector('img'));
    const hasRecall = items.some((el) => /撤回了一条消息/.test(el.textContent || ''));
    expect(hasImg).toBe(true);
    expect(hasRecall).toBe(true);
  });
});

describe('输入框定位（结构启发式，不依赖 data-e2e）', () => {
  it('contenteditable 输入框可定位', () => {
    document.body.innerHTML = '<div class="editor"><div contenteditable="true">输入...</div></div>';
    const input = SelectorEngine.locateInput(document);
    expect(input).not.toBeNull();
    expect(input.getAttribute('contenteditable')).toBe('true');
  });
});
