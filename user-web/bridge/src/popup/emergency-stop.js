// Bridge 紧急停止（Kill Switch，2026-08-15 M2-P1-产品3）
//
// 目标：用户点一下「紧急停止」按钮 → 全局立刻停所有桥接活动：
//   - 通过 chrome.storage.local 写入 emergencyStop=true
//   - 广播到所有 content script：立即停止巡检、停所有 HTTP 请求、清空 pendingAck
//   - popup UI 立刻显示"已停止"状态，再次点击"恢复"按钮清除标记
//
// 设计原则：
//   - 不依赖 background 实时通信（kill switch 必须立即生效）
//   - storage 即配置：content 每次发请求前查 storage，命中则跳过
//   - 不可逆 vs 可恢复：本开关可恢复（写入 false 即解除）

import { createLogger } from '../core/logger.js';

const log = createLogger('killswitch', 'popup');

const KEY_EMERGENCY = 'bridgeEmergencyStop';

// 读取当前是否处于紧急停止
export function isEmergencyStop() {
  return new Promise((resolve) => {
    try {
      chrome.storage.local.get(KEY_EMERGENCY, (res) => {
        try {
          if (chrome.runtime.lastError) {
            resolve(false);
            return;
          }
        } catch (_) { /* noop */ }
        resolve(res[KEY_EMERGENCY] === true);
      });
    } catch (_) {
      resolve(false);
    }
  });
}

// 触发紧急停止
export function triggerEmergencyStop(reason) {
  return new Promise((resolve) => {
    try {
      chrome.storage.local.set({ [KEY_EMERGENCY]: true }, () => {
        const err = (() => { try { return chrome.runtime.lastError; } catch (_) { return null; } })();
        if (err) {
          log.error('紧急停止写入失败：' + err.message);
          resolve({ ok: false, error: err.message });
          return;
        }
        log.warn('紧急停止已触发', { reason: reason || 'manual' });
        // 通知 background 广播到所有 content（best-effort，不阻塞）
        try {
          chrome.runtime.sendMessage({ type: 'emergencyStop', reason: reason || 'manual' }, () => {
            try { void chrome.runtime.lastError; } catch (_) { /* noop */ }
          });
        } catch (_) { /* noop */ }
        resolve({ ok: true });
      });
    } catch (e) {
      resolve({ ok: false, error: String(e) });
    }
  });
}

// 恢复桥接（清除紧急停止标记）
export function resumeBridge() {
  return new Promise((resolve) => {
    try {
      chrome.storage.local.set({ [KEY_EMERGENCY]: false }, () => {
        const err = (() => { try { return chrome.runtime.lastError; } catch (_) { return null; } })();
        if (err) {
          resolve({ ok: false, error: err.message });
          return;
        }
        log.info('紧急停止已恢复');
        // 通知 background 广播到所有 content
        try {
          chrome.runtime.sendMessage({ type: 'resumeBridge' }, () => {
            try { void chrome.runtime.lastError; } catch (_) { /* noop */ }
          });
        } catch (_) { /* noop */ }
        resolve({ ok: true });
      });
    } catch (e) {
      resolve({ ok: false, error: String(e) });
    }
  });
}

// 切换紧急停止状态（返回新的状态）
export async function toggleEmergencyStop() {
  const current = await isEmergencyStop();
  if (current) {
    await resumeBridge();
    return false;
  }
  await triggerEmergencyStop('manual_toggle');
  return true;
}

// 测试钩子
if (typeof window !== 'undefined') {
  window.__emergencyStop = {
    isEmergencyStop,
    triggerEmergencyStop,
    resumeBridge,
    toggleEmergencyStop,
  };
}
