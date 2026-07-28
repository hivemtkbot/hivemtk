// DOM 交互工具：逐行移植自三个开源仓库的真实交互逻辑。
// - simulateRealClick / humanType：来自 DY-auto（抖音）
// - enhancedClickWithVerification / setValue：来自 XHS-YYDS（小红书）
// - simulateTyping / simulateEnterKey / simulateRealClick：来自 tiktok-auto-plugin
import { createLogger } from './logger.js';

export { createLogger };

const log = createLogger('dom', 'bridge');

export const qs = (sel, root = document) => root.querySelector(sel);
export const qsa = (sel, root = document) => Array.from(root.querySelectorAll(sel));

/** 等待某个 selector 出现（抖音自动回复脚本用的 waitForElement 同款思路） */
export function waitFor(selector, { timeout = 8000, root = document } = {}) {
  return new Promise((resolve) => {
    const el = qs(selector, root);
    if (el) return resolve(el);
    const start = Date.now();
    const timer = setInterval(() => {
      const found = qs(selector, root);
      if (found || Date.now() - start > timeout) {
        clearInterval(timer);
        resolve(found || null);
      }
    }, 200);
  });
}

/** 真实点击：合成完整 Pointer + Mouse 事件序列（DY-auto / tiktok 同款，绕过点击拦截） */
export function simulateRealClick(element) {
  if (!element) return;
  const rect = element.getBoundingClientRect();
  const x = rect.left + rect.width / 2;
  const y = rect.top + rect.height / 2;
  const opts = (type) => ({
    view: window,
    bubbles: true,
    cancelable: true,
    clientX: x,
    clientY: y,
    screenX: x,
    screenY: y,
    button: 0,
    buttons: 1,
    composed: true,
    pointerId: 1,
    pointerType: 'mouse',
    isPrimary: true,
  });
  element.dispatchEvent(new PointerEvent('pointerdown', opts('pointerdown')));
  element.dispatchEvent(new MouseEvent('mousedown', opts('mousedown')));
  element.dispatchEvent(new PointerEvent('pointerup', opts('pointerup')));
  element.dispatchEvent(new MouseEvent('mouseup', opts('mouseup')));
  element.dispatchEvent(new MouseEvent('click', opts('click')));
}

/**
 * 小红书同款点击：先原生 click，再补一层 mousedown/up/click 事件。
 * 用于 .send_btn 这类需要真实事件触发的按钮。
 */
export function enhancedClick(element) {
  if (!element) return;
  try {
    element.click();
  } catch (e) {
    log.debug('native click failed, fallback', e);
  }
  const ev = (type) =>
    new MouseEvent(type, { bubbles: true, cancelable: true, view: window });
  element.dispatchEvent(ev('mousedown'));
  element.dispatchEvent(ev('mouseup'));
  element.dispatchEvent(ev('click'));
}

/** 小红书 setValue：textarea 设值 + 触发 input（domUtils.js 同款） */
export function setValue(el, value) {
  if (!el) return;
  const proto = el.tagName === 'TEXTAREA' ? window.HTMLTextAreaElement.prototype : window.HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
  if (setter) setter.call(el, value);
  else el.value = value;
  el.dispatchEvent(new Event('input', { bubbles: true }));
  el.dispatchEvent(new Event('change', { bubbles: true }));
}

/**
 * 抖音 fillInputViaPaste：contenteditable 填值（粘贴事件优先，保证框架识别输入）。
 * 参考 DY-auto fillInputViaPaste：触发 paste + 写入 innerText + 派发 input。
 */
export function fillContentEditable(el, text) {
  if (!el) return;
  el.focus();
  // 1) 原生 execCommand 输入（最贴近真人，抖音/小红书 contenteditable 均认）
  try {
    document.execCommand('insertText', false, text);
  } catch (e) {
    /* noop */
  }
  // 2) 兜底：直接写 innerText + 派发 input 事件
  if ((el.innerText || '').trim() !== text.trim()) {
    el.innerText = text;
    el.dispatchEvent(new InputEvent('input', { bubbles: true }));
    el.dispatchEvent(new Event('input', { bubbles: true }));
  }
}

/** 抖音 humanType：逐字符 execCommand('insertText')（更拟人，适合作者打字节奏） */
export function humanType(el, text, { interval = 20 } = {}) {
  return new Promise((resolve) => {
    if (!el) return resolve();
    el.focus();
    let i = 0;
    const tick = () => {
      if (i >= text.length) return resolve();
      try {
        document.execCommand('insertText', false, text[i]);
      } catch (e) {
        el.innerText = (el.innerText || '') + text[i];
      }
      i += 1;
      setTimeout(tick, interval);
    };
    tick();
  });
}

/**
 * TikTok simulateTyping：先聚焦，再逐字符写值并派发 input（适配 Draft.js 受控输入）。
 * 对 textarea / contenteditable 均兼容。
 */
export function simulateTyping(el, text, { interval = 25 } = {}) {
  return new Promise((resolve) => {
    if (!el) return resolve();
    el.focus();
    let acc = '';
    let i = 0;
    const tick = () => {
      if (i >= text.length) {
        el.dispatchEvent(new Event('input', { bubbles: true }));
        return resolve();
      }
      acc += text[i];
      if (el.tagName === 'TEXTAREA' || el.tagName === 'INPUT') {
        const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value')?.set;
        if (setter) setter.call(el, acc);
        else el.value = acc;
      } else {
        try {
          document.execCommand('insertText', false, text[i]);
        } catch (e) {
          el.innerText = acc;
        }
      }
      el.dispatchEvent(new InputEvent('input', { bubbles: true }));
      i += 1;
      setTimeout(tick, interval);
    };
    tick();
  });
}

/** TikTok simulateEnterKey：在输入框派发回车（Draft.js 靠 Enter 发送） */
export function simulateEnterKey(el) {
  if (!el) return;
  const ev = (type) =>
    new KeyboardEvent(type, { key: 'Enter', code: 'Enter', keyCode: 13, which: 13, bubbles: true, cancelable: true });
  el.dispatchEvent(ev('keydown'));
  el.dispatchEvent(ev('keypress'));
  el.dispatchEvent(ev('keyup'));
}

/** 取纯文本并压缩多余空白 */
export function cleanText(el) {
  if (!el) return '';
  return (el.innerText || el.textContent || '').replace(/\s+/g, ' ').trim();
}

/** 包裹 MutationObserver 的便捷 API */
export function observe(root, cb, options = { childList: true, subtree: true }) {
  if (!root) return null;
  const obs = new MutationObserver(cb);
  obs.observe(root, options);
  return obs;
}
