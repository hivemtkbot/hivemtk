// Bridge Popup 告警横幅监听器（2026-08-15 M2-P1-产品5）
//
// 目标：popup 打开时持续监听 health 状态，熔断器 OPEN / 死开关触发时
//       自动在顶部展示红色横幅，避免用户看不到"桥接已挂"的信号。
//
// 与 health.js 联动：复用 fetchHealth() + detectHealthAlert()。
//
// 使用：
//   import { startAlertPolling, stopAlertPolling } from './alert-banner.js';
//   const stop = startAlertPolling({ onAlert: (alert) => showBanner(alert.level, alert.title, alert.body) });
//   // 关闭 popup 时：stopAlertPolling(stop);

import { fetchHealth, detectHealthAlert } from './health.js';
import { UI_DEFAULTS } from '../core/constants.js';

let _stopFn = null;
let _lastAlertKey = null; // 用于去重：相同告警不重复弹

// 启动告警轮询
// opts: { onAlert, onClear, intervalMs? }
//   onAlert(alert): 检测到告警时调用（alert = { level, channel, title, body }）
//   onClear():      告警消除时调用（之前有告警，现在无）
// 返回 stop 函数
export function startAlertPolling(opts) {
  // 清理旧轮询
  if (_stopFn) {
    try { _stopFn(); } catch (_) { /* noop */ }
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
    } catch (_) { /* noop */ }
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
    try { _stopFn(); } catch (_) { /* noop */ }
    _stopFn = null;
  }
}

// 测试钩子
if (typeof window !== 'undefined') {
  window.__alertBanner = {
    startAlertPolling,
    stopAlertPolling,
    _getLastAlertKey: () => _lastAlertKey,
  };
}
