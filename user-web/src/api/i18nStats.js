import { http } from '@/utils/request';

export function getI18nStats() {
  return http.get('/api/i18n/stats')
}

export function getLangDistribution(days = 7) {
  return http.get('/api/i18n/stats/lang-dist', { params: { days } })
}

export function getCacheHitRate(days = 7) {
  return http.get('/api/i18n/stats/cache', { params: { days } })
}

export function getGlossaryCoverage() {
  return http.get('/api/i18n/stats/glossary')
}

export function getQualityTrend(days = 30) {
  return http.get('/api/i18n/stats/quality', { params: { days } })
}

export function getLatencyStats(days = 7) {
  return http.get('/api/i18n/stats/latency', { params: { days } })
}

export default {
  getI18nStats,
  getLangDistribution,
  getCacheHitRate,
  getGlossaryCoverage,
  getQualityTrend,
  getLatencyStats
}
