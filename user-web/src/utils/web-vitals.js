/**
 * Web Vitals 监控（OPT-FE-14）
 *
 * 上报端点：POST /api/monitor/web-vitals
 * 传输方式：navigator.sendBeacon()（无阻塞、可携带 cookie、可在 unload 时下发）
 * 指标覆盖：CLS / FID / LCP / FCP / TTFB（web-vitals v4 API）
 *
 * 注意：
 *   1. 仅在生产构建（import.meta.env.PROD）下上报；
 *   2. 上报失败/超时静默回退到 navigator.sendBeacon 队列；
 *   3. 与前端监控后端约定 payload 字段：
 *      { name, value, id, rating, navigationType, page, ts, ua }
 */

import { onCLS, onFCP, onFID, onLCP, onTTFB } from 'web-vitals'

const REPORT_URL = '/api/monitor/web-vitals'
const PROD_ONLY = true
const SAMPLING_RATE = 0.1 // 仅上报 10% 页面访问，避免高 QPS 拖垮监控后端

/**
 * sendBeacon 优先；fallback 到 fetch keepalive
 */
function report(payload) {
  try {
    const body = JSON.stringify(payload)
    if (navigator.sendBeacon) {
      const blob = new Blob([body], { type: 'application/json' })
      const ok = navigator.sendBeacon(REPORT_URL, blob)
      if (ok) return
    }
    // fallback
    fetch(REPORT_URL, {
      method: 'POST',
      body,
      headers: { 'Content-Type': 'application/json' },
      keepalive: true,
    }).catch(() => {})
  } catch (_) {
    // 静默失败
  }
}

function getRating(name, value) {
  // web-vitals v4: onCLS/onLCP/... 第二个参数 callback 拿到的 metric 已自带 rating
  // 这里仅在 metric 未提供时做兜底
  switch (name) {
    case 'CLS':
      return value <= 0.1 ? 'good' : value <= 0.25 ? 'needs-improvement' : 'poor'
    case 'FID':
      return value <= 100 ? 'good' : value <= 300 ? 'needs-improvement' : 'poor'
    case 'LCP':
      return value <= 2500 ? 'good' : value <= 4000 ? 'needs-improvement' : 'poor'
    case 'FCP':
      return value <= 1800 ? 'good' : value <= 3000 ? 'needs-improvement' : 'poor'
    case 'TTFB':
      return value <= 800 ? 'good' : value <= 1800 ? 'needs-improvement' : 'poor'
    default:
      return 'unknown'
  }
}

function buildPayload(metric) {
  return {
    name: metric.name,
    value: metric.value,
    id: metric.id,
    rating: metric.rating || getRating(metric.name, metric.value),
    navigationType: metric.navigationType || 'navigate',
    page: location.pathname + location.search,
    ts: Date.now(),
    ua: navigator.userAgent,
    // 调试钩子
    dev: import.meta.env.DEV,
  }
}

let initialized = false

export function initWebVitals() {
  if (initialized) return
  initialized = true

  if (PROD_ONLY && !import.meta.env.PROD) return
  if (typeof window === 'undefined') return

  // 采样：Math.random() < SAMPLING_RATE 才上报
  if (Math.random() >= SAMPLING_RATE) return

  const onUpdate = (metric) => {
    report(buildPayload(metric))
  }

  try {
    onCLS(onUpdate, { reportAllChanges: true })
    onFCP(onUpdate)
    onFID(onUpdate)
    onLCP(onUpdate)
    onTTFB(onUpdate)
  } catch (e) {
    console.warn('[web-vitals] init failed:', e)
  }
}

export default initWebVitals
