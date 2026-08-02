import { http } from '@/utils/request'

// 意图识别 API - 匹配后端 /api/intent/* 路径

/**
 * @typedef {Object} RecognizeParams
 * @property {string} message
 * @property {string} [context]
 * @property {string} [customer_id]
 * @property {string} [platform]
 */

/**
 * @typedef {Object} BatchRecognizeParams
 * @property {string[]} messages
 */

/**
 * @typedef {Object} IntentStatsParams
 * @property {number} [page]
 * @property {number} [page_size]
 * @property {string} [start_date]
 * @property {string} [end_date]
 */

/**
 * @typedef {Object} IntentRecentParams
 * @property {number} [page]
 * @property {number} [page_size]
 * @property {string} [intent_type]
 */

export const intentApi = {
  // 单条识别（13 大类销售意图：规则 + LLM 兜底）
  recognize(data) {
    return http.post('/api/intent/recognize', data)
  },

  // 批量识别（兼容 {items:[{text,...}]} 和 {messages:["msg",...]} 两种入参）
  batchRecognize(data) {
    return http.post('/api/intent/recognize/batch', data)
  },

  // 意图统计（返回 {total, distribution, by_intent, by_method, by_level, period_days}）
  // params: { days: 7 }
  getStats(params) {
    return http.get('/api/intent/stats', params)
  },

  // 最近意图（分页 + 意图类型筛选）
  // params: { page, page_size, customer_id, intent_type }
  getRecent(params) {
    return http.get('/api/intent/recent', params)
  },

  // 意图词典
  getDict() {
    return http.get('/api/intent/dict')
  },

  // ===== 精细意图识别（8 大类 + 7 子类）=====

  // 精细识别：返回 {intent_major, intent_minor, confidence, ...}
  recognizeFine(data) {
    return http.post('/api/intent/recognize/fine', data)
  },

  // 精细识别日志（params: { customer_id?, major?, limit? }）
  getLogs(params) {
    return http.get('/api/intent/logs', params)
  },

  // 精细识别统计（params: { days? }）
  getStatsFine(params) {
    return http.get('/api/intent/stats/fine', params)
  },

  // ===== 意图识别配置管理（前端在线开关）=====

  /**
   * 获取意图识别配置
   * @returns {Promise<{enabled: boolean, updated_at?: string, updated_by?: string}>}
   */
  getConfig() {
    return http.get('/api/intent/config')
  },

  /**
   * 更新意图识别配置
   * @param {{enabled: boolean}} data
   * @returns {Promise<{enabled: boolean, updated_at: string, updated_by: string}>}
   */
  updateConfig(data) {
    return http.put('/api/intent/config', data)
  }
}
