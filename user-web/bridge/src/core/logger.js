// 轻量日志（按频道着色，便于真机校准时排查）
//
// 默认参数：MAX_LOG_CHARS = SECURITY.logMaskMaxChars（见 constants.js / DEFAULTS.md）
//   - 调整后仅改 constants.js 即可
import { SECURITY } from './constants.js';

const COLORS = {
  douyin: '#FE2C55',
  xhs: '#ff2442',
  tiktok: '#25F4EE',
  bridge: '#4f8cff',
  bg: '#888',
};

// 质量: 日志脱敏
// 日志中禁止打印完整 content / message（可能含 PII / 隐私）。
// 策略：自动把"过长字符串"或典型 content 字段截断到 MAX_LOG_CHARS + "..."，
//       避免误打全量对话内容到控制台（控制台可被同浏览器其他扩展读取）。
const MAX_LOG_CHARS = SECURITY.logMaskMaxChars;
const SENSITIVE_KEYS = new Set(['content', 'message', 'text', 'reply_text', 'msg_content']);

function maskString(s) {
  if (typeof s !== 'string') return s;
  if (s.length <= MAX_LOG_CHARS) return s;
  return s.slice(0, MAX_LOG_CHARS) + '...';
}

function sanitizeArgs(args) {
  return args.map((a) => {
    if (a == null) return a;
    if (typeof a === 'string') return maskString(a);
    if (typeof a === 'object') {
      try {
        const out = Array.isArray(a) ? [] : {};
        for (const k of Object.keys(a)) {
          if (SENSITIVE_KEYS.has(k) && typeof a[k] === 'string') {
            out[k] = maskString(a[k]);
          } else {
            out[k] = a[k];
          }
        }
        return out;
      } catch (e) {
        return '[unserializable]';
      }
    }
    return a;
  });
}

export function createLogger(tag, channel) {
  const color = COLORS[channel] || COLORS[tag] || COLORS.bridge;
  const prefix = `%c[bridge:${tag}]`;
  const style = `color:${color};font-weight:bold`;
  return {
    debug: (...a) => console.debug(prefix, style, ...sanitizeArgs(a)),
    info: (...a) => console.log(prefix, style, ...sanitizeArgs(a)),
    warn: (...a) => console.warn(prefix, style, ...sanitizeArgs(a)),
    error: (...a) => console.error(prefix, style, ...sanitizeArgs(a)),
  };
}
