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
  // 单条识别
  recognize(data) {
    return http.post('/api/intent/recognize', data)
  },

  // 批量识别
  batchRecognize(data) {
    return http.post('/api/intent/recognize/batch', data)
  },

  // 意图统计（params: page, page_size, start_date, end_date）
  getStats(params) {
    return http.get('/api/intent/stats', params)
  },

  // 最近意图（params: page, page_size, intent_type）
  getRecent(params) {
    return http.get('/api/intent/recent', params)
  },

  // 意图字典
  getDict() {
    return http.get('/api/intent/dict')
  }
}
