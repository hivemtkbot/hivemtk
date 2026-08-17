import { http } from '@/utils/request';

// ============================================================================
// 多语言（I18n）监控统计 API
// ----------------------------------------------------------------------------
// 对应后端 /api/i18n/stats 路由，用于多语言翻译服务的运行态监控看板。
// ============================================================================

// 总览：{ total_calls, cross_lingual_calls, cache_hit_rate, fallback_rate, avg_quality }
export function getI18nStats() {
  return http.get('/api/i18n/stats')
}

// 语言分布：[{ internal_lang, target_lang, count, cross_lingual_count }]
// params: { days }
export function getLangDistribution(days = 7) {
  return http.get('/api/i18n/stats/lang-dist', { params: { days } })
}

// 缓存命中率：{ hit, miss, hit_rate }
// params: { days }
export function getCacheHitRate(days = 7) {
  return http.get('/api/i18n/stats/cache', { params: { days } })
}

// 术语覆盖率：[{ target_lang, term_count, active_count }]
export function getGlossaryCoverage() {
  return http.get('/api/i18n/stats/glossary')
}

// 质量评分趋势：[{ date, avg_score, total_count }]
// params: { days }
export function getQualityTrend(days = 30) {
  return http.get('/api/i18n/stats/quality', { params: { days } })
}

// 延迟统计：[{ target_lang, p50, p95, p99, count }]
// params: { days }
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
