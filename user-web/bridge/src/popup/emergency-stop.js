
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
        } catch (_) {  }
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
        try {
          chrome.runtime.sendMessage({ type: 'emergencyStop', reason: reason || 'manual' }, () => {
            try { void chrome.runtime.lastError; } catch (_) {  }
          });
        } catch (_) {  }
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
        try {
          chrome.runtime.sendMessage({ type: 'resumeBridge' }, () => {
            try { void chrome.runtime.lastError; } catch (_) {  }
          });
        } catch (_) {  }
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

if (typeof window !== 'undefined') {
  window.__emergencyStop = {
    isEmergencyStop,
    triggerEmergencyStop,
    resumeBridge,
    toggleEmergencyStop,
  };
}

