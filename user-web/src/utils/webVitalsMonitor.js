import { onLCP, onFID, onCLS, onTTFB, onINP } from 'web-vitals';

const REPORT_URL = '/api/monitor/web-vitals'

export const initWebVitals = (options = {}) => {
  const reportTo = options.endpoint || REPORT_URL
  const sampleRate = options.sampleRate || 1.0
  const debug = options.debug || false

  if (Math.random() > sampleRate)
    return;

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
    if (navigator.sendBeacon) {
      navigator.sendBeacon(reportTo, JSON.stringify(data))
    } else if (typeof fetch !== 'undefined') {
      fetch(reportTo, { method: 'POST', body: JSON.stringify(data), keepalive: true }).catch(() => {})
    }
  }

  onLCP(send);
  onFID(send)
  onCLS(send)
  onTTFB(send)
  if (onINP)
    onINP(send);
};

export const reportCustomMetric = (name, value, metadata = {}) => {
  if (typeof fetch === 'undefined') return
  fetch('/api/monitor/web-vitals', {
    method: 'POST',
    body: JSON.stringify({ name, value, ...metadata, custom: true, timestamp: Date.now() }),
    keepalive: true
  }).catch(() => {})
};

export default { initWebVitals, reportCustomMetric }
