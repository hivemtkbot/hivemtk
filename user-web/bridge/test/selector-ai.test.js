// selector-ai 单元测试：选择器配置中心（纯规则，无 LLM）
//
// 设计（用户诉求）：去掉 LLM 抽取结构逻辑，改回「人工配置选择器」——
// 开发者从 DevTools 识别 HTML 关键 class，填写进 popup 配置（chrome.storage/localStorage），
// 扩展运行时用户配置优先、渠道默认 SEL 兜底。无需发版、无云端依赖。
import { describe, it, expect, beforeEach } from 'vitest';
import {
  mergeSelectors,
  saveSelectors,
  getCustomSelectors,
  clearExtractorResultCache,
  SELECTOR_FIELDS,
  SELECTOR_CONFIG_KEY,
} from '../src/core/selector-ai.js';

const CHANNEL = 'douyin_web';

// 渠道默认 SEL 兜底（模拟 channels/douyin.js 传入的规则）
const FB = {
  itemSelectors: ['[data-e2e="msg-item-content"]', '.bubble'],
  listSelectors: ['.im-message-list'],
  textSelectors: ['.text'],
  inputSelectors: ['div[contenteditable]'],
  sendSelectors: ['[class*="send"]'],
  selfMarkers: ['.self'],
  otherMarkers: ['.other'],
};

beforeEach(() => {
  try { localStorage.removeItem(SELECTOR_CONFIG_KEY); } catch (_) { /* noop */ }
  clearExtractorResultCache(CHANNEL);
});

describe('mergeSelectors 纯规则合并', () => {
  it('无用户配置 → 仅返回规则兜底（SEL 默认）', () => {
    const m = mergeSelectors(CHANNEL, FB);
    expect(m.itemSelectors).toEqual(['[data-e2e="msg-item-content"]', '.bubble']);
    expect(m.listSelectors).toEqual(['.im-message-list']);
    expect(m.inputSelectors).toEqual(['div[contenteditable]']);
  });

  it('有用户配置 → 用户选择器在前、规则兜底在后（覆盖优先级）', async () => {
    await saveSelectors({
      [CHANNEL]: {
        messageItem: '.xhs-im-msg-item, .chat-item',
        messageList: '.xhs-im-msg-list',
        input: 'div.xhs-im-input-bar-editor[contenteditable]',
        send: '.xhs-im-send',
      },
    });
    const m = mergeSelectors(CHANNEL, FB);
    // 用户配置在前
    expect(m.itemSelectors[0]).toBe('.xhs-im-msg-item');
    expect(m.itemSelectors[1]).toBe('.chat-item');
    // 规则兜底在后
    expect(m.itemSelectors).toContain('.bubble');
    expect(m.listSelectors[0]).toBe('.xhs-im-msg-list');
    expect(m.inputSelectors[0]).toBe('div.xhs-im-input-bar-editor[contenteditable]');
    expect(m.sendSelectors[0]).toBe('.xhs-im-send');
    // 未填字段仍用默认
    expect(m.textSelectors).toEqual(['.text']);
  });

  it('用户配置与默认重复 → 去重', async () => {
    await saveSelectors({
      [CHANNEL]: { messageItem: '.bubble, [data-e2e="msg-item-content"]' },
    });
    const m = mergeSelectors(CHANNEL, FB);
    expect(m.itemSelectors).toEqual(['.bubble', '[data-e2e="msg-item-content"]']);
    expect(m.itemSelectors.filter((s) => s === '.bubble')).toHaveLength(1);
  });

  it('不同渠道配置隔离（A 渠道配置不影响 B 渠道）', async () => {
    await saveSelectors({ xhs_web: { messageItem: '.xhs-im-msg-item' } });
    const m = mergeSelectors('douyin_web', FB);
    expect(m.itemSelectors).toEqual(['[data-e2e="msg-item-content"]', '.bubble']);
  });

  it('清空配置后回退默认（删除存储键）', async () => {
    await saveSelectors({ [CHANNEL]: { messageItem: '.custom-item' } });
    expect(mergeSelectors(CHANNEL, FB).itemSelectors[0]).toBe('.custom-item');
    try { localStorage.removeItem(SELECTOR_CONFIG_KEY); } catch (_) { /* noop */ }
    clearExtractorResultCache(CHANNEL);
    expect(mergeSelectors(CHANNEL, FB).itemSelectors[0]).toBe('[data-e2e="msg-item-content"]');
  });
});

describe('选择器配置持久化', () => {
  it('saveSelectors → getCustomSelectors 往返一致', async () => {
    await saveSelectors({ [CHANNEL]: { messageItem: '.chat-item', messageList: '.xhs-im-msg-list' } });
    const cfg = getCustomSelectors(CHANNEL);
    expect(cfg.messageItem).toBe('.chat-item');
    expect(cfg.messageList).toBe('.xhs-im-msg-list');
  });

  it('无配置 → getCustomSelectors 返回 null', () => {
    try { localStorage.removeItem(SELECTOR_CONFIG_KEY); } catch (_) { /* noop */ }
    clearExtractorResultCache(CHANNEL);
    expect(getCustomSelectors(CHANNEL)).toBeNull();
  });

  it('SELECTOR_FIELDS 覆盖所有关键节点', () => {
    expect(SELECTOR_FIELDS).toContain('conversationList');
    expect(SELECTOR_FIELDS).toContain('messageItem');
    expect(SELECTOR_FIELDS).toContain('messageList');
    expect(SELECTOR_FIELDS).toContain('text');
    expect(SELECTOR_FIELDS).toContain('input');
    expect(SELECTOR_FIELDS).toContain('send');
    expect(SELECTOR_FIELDS).toContain('selfMarker');
    expect(SELECTOR_FIELDS).toContain('otherMarker');
  });
});
