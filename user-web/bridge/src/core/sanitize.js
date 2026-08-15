
import { SECURITY } from './constants.js';

const HTML_ESCAPE_MAP = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
  '/': '&#x2F;',
  '`': '&#x60;',
  '=': '&#x3D;',
};

export function escapeHTML(s) {
  if (s == null) return '';
  return String(s).replace(/[&<>"'`=/]/g, (ch) => HTML_ESCAPE_MAP[ch] || ch);
}

// 净化用户控制文本：去掉可能的 script/iframe 注入、控制字符、超长截断
// 用于限制单条 AI 回复最大字节数（防止恶意 prompt 注入 + 平台显示限制）
// 文档源：handler.go: maxReplyContentBytes = 4 * 1024；前端 constants.SECURITY.maxReplyContentBytes
//        测试：test/constants.test.js "maxReplyContentBytes 必须与服务端 handler.go ... 严格对齐"
export const MAX_BODY_BYTES = SECURITY.maxReplyContentBytes;

export function sanitizeForDisplay(text, maxBytes = MAX_BODY_BYTES) {
  if (text == null) return '';
  let s = String(text);
  // 有意移除 C0 控制字符（防注入 + 平台显示限制），此处控制字符属安全净化的目标而非误用
  // eslint-disable-next-line no-control-regex
  s = s.replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/g, '');
  if (s.length > maxBytes) s = s.slice(0, maxBytes);
  return s;
}

// 安全设值到 contenteditable 容器：使用 textContent 而非 innerHTML
//   - 防止 AI 回复中携带 <script> 之类 XSS payload 触发
//   - 平台 IM 容器通常用 contenteditable 渲染，textContent 即可保留换行
export function safeSetContent(node, text) {
  if (!node) return;
  const safe = sanitizeForDisplay(text);
  node.textContent = safe;
  node.dispatchEvent(new Event('input', { bubbles: true }));
}

// 安全设值到 textarea / input 元素
export function safeSetValue(el, text) {
  if (!el) return;
  const safe = sanitizeForDisplay(text);
  const proto =
    el.tagName === 'TEXTAREA'
      ? window.HTMLTextAreaElement.prototype
      : el.tagName === 'INPUT'
        ? window.HTMLInputElement.prototype
        : null;
  if (proto) {
    const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
    if (setter) setter.call(el, safe);
    else el.value = safe;
  } else {
    el.value = safe;
  }
  el.dispatchEvent(new Event('input', { bubbles: true }));
  el.dispatchEvent(new Event('change', { bubbles: true }));
}

