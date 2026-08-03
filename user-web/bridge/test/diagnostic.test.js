// 诊断相关单测：重点回归「SVG 发送按钮被 isVisible 误判不可见」导致自检报 0 发送按钮。
// 真实背景：抖音发送按钮是 svg.messageMsgInputpublishBtn.e2e-send-msg-btn，
// SVG 的 offsetParent 恒为 null，旧 isVisible 据此判隐藏 → 自检「发送按钮 0 个」。
import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { isVisible, scanDomSnapshot } from '../src/content/common.js';

// jsdom 下 getBoundingClientRect 默认全 0、HTMLElement.offsetParent 恒为 null（不做布局），
// 元素一律被判不可见；统一 stub 为非零尺寸 + 非 null offsetParent（仅 HTML，SVG 不受影响）。
const realGbcr = window.Element.prototype.getBoundingClientRect;
const stubRect = () => ({ x: 0, y: 0, width: 200, height: 40, top: 0, left: 0, right: 200, bottom: 40 });
const realOffsetParent = Object.getOwnPropertyDescriptor(window.HTMLElement.prototype, 'offsetParent');

beforeEach(() => {
  window.Element.prototype.getBoundingClientRect = stubRect;
  Object.defineProperty(window.HTMLElement.prototype, 'offsetParent', { configurable: true, get: () => document.body });
  document.body.innerHTML = '';
});

afterEach(() => {
  window.Element.prototype.getBoundingClientRect = realGbcr;
  if (realOffsetParent) Object.defineProperty(window.HTMLElement.prototype, 'offsetParent', realOffsetParent);
  else delete window.HTMLElement.prototype.offsetParent;
  document.body.innerHTML = '';
});

describe('isVisible（SVG 安全）', () => {
  it('SVG 发送按钮（offsetParent 恒为 null）应判为可见', () => {
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('class', 'messageMsgInputpublishBtn e2e-send-msg-btn');
    document.body.appendChild(svg);
    expect(isVisible(svg)).toBe(true);
  });

  it('display:none 的 SVG 仍判为不可见', () => {
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('class', 'e2e-send-msg-btn');
    svg.style.display = 'none';
    document.body.appendChild(svg);
    expect(isVisible(svg)).toBe(false);
  });

  it('普通隐藏 div（offsetParent null 且非 fixed）仍判为不可见', () => {
    const div = document.createElement('div');
    div.style.display = 'none';
    document.body.appendChild(div);
    expect(isVisible(div)).toBe(false);
  });
});

describe('scanDomSnapshot（抖音 /jingxuan 浮层）', () => {
  it('应检测到 svg 发送按钮 + 输入框 + 会话列表', () => {
    // 1) 真实输入框：zone-container.editor-kit-container.messageEditorinputArea
    const editor = document.createElement('div');
    editor.className = 'zone-container editor-kit-container messageEditorinputArea';
    editor.setAttribute('contenteditable', 'true');
    document.body.appendChild(editor);

    // 2) 会话列表 + 会话项
    const list = document.createElement('div');
    list.className = 'conversationConversationListwrapper';
    const item = document.createElement('div');
    item.setAttribute('data-e2e', 'conversation-item');
    list.appendChild(item);
    document.body.appendChild(list);

    // 3) 真实 svg 发送按钮
    const send = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    send.setAttribute('class', 'messageMsgInputpublishBtn e2e-send-msg-btn');
    document.body.appendChild(send);

    const snap = scanDomSnapshot();
    expect(snap.inputCount).toBeGreaterThanOrEqual(1);
    expect(snap.listRootCount).toBeGreaterThanOrEqual(1);
    // 关键回归：svg 发送按钮须被纳入「发送按钮」计数（旧版会因 offsetParent=null 漏掉）
    expect(snap.sendBtnCount).toBeGreaterThanOrEqual(1);
  });
});
