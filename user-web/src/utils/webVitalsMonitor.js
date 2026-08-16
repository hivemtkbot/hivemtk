/**
 * 首屏 LCP 监控（USR-PF-03）
 * 依赖：web-vitals 4.2.4（已在 package.json）
 * 借鉴：https://web.dev/lcp/
 */

import { onLCP, onFID, onCLS, onTTFB, onINP } from 'web-vitals'

const REPORT_URL = '/api/monitor/web-vitals'

/**
 * 初始化 web-vitals 上报
 */
export const initWebVitals = (options = {}) => {
  const reportTo = options.endpoint || REPORT_URL
  const sampleRate = options.sampleRate || 1.0
  const debug = options.debug || false

  if (Math.random() > sampleRate) return // 采样

  const send = (metric) => {
    const data = {
      name: metric.name,
      value: metric.value,
      id: metric.id,
      rating: metric.rating,
      delta: metric.delta,
      navigationType: metric.navigationType,
      url: location.href,
      userAgent: navigator.userAgent,
      timestamp: Date.now()
    }
    if (debug) console.log('[web-vitals]', data)
    // sendBeacon 上报（页面关闭时也能发送）
    if (navigator.sendBeacon) {
      navigator.sendBeacon(reportTo, JSON.stringify(data))
    } else if (typeof fetch !== 'undefined') {
      fetch(reportTo, { method: 'POST', body: JSON.stringify(data), keepalive: true }).catch(() => {})
    }
  }

  // 注册所有关键指标
  onLCP(send)
  onFID(send)
  onCLS(send)
  onTTFB(send)
  if (onINP) onINP(send) // INP 替代 FID（web-vitals v4）
}

/**
 * 手动报告自定义指标
 */
export const reportCustomMetric = (name, value, metadata = {}) => {
  if (typeof fetch === 'undefined') return
  fetch('/api/monitor/web-vitals', {
    method: 'POST',
    body: JSON.stringify({ name, value, ...metadata, custom: true, timestamp: Date.now() }),
    keepalive: true
  }).catch(() => {})
}

export default { initWebVitals, reportCustomMetric }
