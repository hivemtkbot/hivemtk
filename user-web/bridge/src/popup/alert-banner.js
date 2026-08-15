
import { fetchHealth, detectHealthAlert } from './health.js';
import { UI_DEFAULTS } from '../core/constants.js';

let _stopFn = null;
let _lastAlertKey = null; 

// 启动告警轮询
// opts: { onAlert, onClear, intervalMs? }
//   onAlert(alert): 检测到告警时调用（alert = { level, channel, title, body }）
//   onClear():      告警消除时调用（之前有告警，现在无）
// 返回 stop 函数
export function startAlertPolling(opts) {
  if (_stopFn) {
    try { _stopFn(); } catch (_) {  }
    _stopFn = null;
  }
  const { onAlert, onClear, intervalMs = UI_DEFAULTS.popupAlertPollMs } = opts || {};
  if (typeof onAlert !== 'function') return () => {};
  let stopped = false;
  const tick = async () => {
    if (stopped) return;
    try {
      const data = await fetchHealth();
      const alert = detectHealthAlert(data);
      // 告警去重：level+channel+title 作为 key
      const alertKey = alert ? `${alert.level}|${alert.channel}|${alert.title}` : null;
      if (alert) {
        if (alertKey !== _lastAlertKey) {
          _lastAlertKey = alertKey;
          onAlert(alert);
        }
      } else if (_lastAlertKey) {
        _lastAlertKey = null;
        if (typeof onClear === 'function') onClear();
      }
    } catch (_) {  }
  };
  tick();
  const timer = setInterval(tick, intervalMs);
  _stopFn = () => {
    stopped = true;
    if (timer) clearInterval(timer);
    _stopFn = null;
    _lastAlertKey = null;
  };
  return _stopFn;
}

// 停止告警轮询
export function stopAlertPolling(stop) {
  if (typeof stop === 'function') stop();
  if (_stopFn) {
    try { _stopFn(); } catch (_) {  }
    _stopFn = null;
  }
}

if (typeof window !== 'undefined') {
  window.__alertBanner = {
    startAlertPolling,
    stopAlertPolling,
    _getLastAlertKey: () => _lastAlertKey,
  };
}

