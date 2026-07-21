import { http } from '@/utils/request'

// 对话记忆 API - 匹配后端 /api/memory/* 路径

/**
 * @typedef {Object} AppendMessageParams
 * @property {string} customer_id
 * @property {string} role
 * @property {string} content
 * @property {Object} [metadata]
 */

/**
 * @typedef {Object} UpdateFactsParams
 * @property {string} customer_id
 * @property {Object} facts
 */

/**
 * @typedef {Object} RecordObjectionParams
 * @property {string} customer_id
 * @property {string} objection
 * @property {string} [response]
 */

/**
 * @typedef {Object} PurchaseIntentParams
 * @property {string} customer_id
 * @property {string} intent_level
 * @property {string[]} [signals]
 */

/**
 * @typedef {Object} IntentTrailParams
 * @property {string} customer_id
 * @property {string} intent
 * @property {number} confidence
 */

/**
 * @typedef {Object} SopHistoryParams
 * @property {string} customer_id
 * @property {string} sop_id
 * @property {string} node_id
 * @property {string} action
 */

/**
 * @typedef {Object} MemoryListParams
 */

/**
 * @typedef {Object} MemoryQueryParams
 * @property {string} customer_id
 * @property {number} [limit]
 */

export const memoryApi = {
  // 追加消息
  appendMessage(data) {
    return http.post('/api/memory/messages', data)
  },

  // 短期记忆（params: customer_id, limit）
  getShortTerm(params) {
    return http.get('/api/memory/short', params)
  },

  // 长期记忆（params: customer_id, limit）
  getLongTerm(params) {
    return http.get('/api/memory/long', params)
  },

  // 更新关键事实
  updateKeyFacts(data) {
    return http.post('/api/memory/facts', data)
  },

  // 记录异议
  recordObjection(data) {
    return http.post('/api/memory/objections', data)
  },

  // 更新购买意向
  updatePurchaseIntent(data) {
    return http.post('/api/memory/purchase-intent', data)
  },

  // 记录意图轨迹
  recordIntent(data) {
    return http.post('/api/memory/intent-trail', data)
  },

  // 记录 SOP 历史
  recordSOP(data) {
    return http.post('/api/memory/sop-history', data)
  },

  // 构建上下文（params: customer_id）
  buildContext(params) {
    return http.get('/api/memory/context', params)
  },

  // 客户记忆列表（params: page, page_size, customer_id）
  list(params) {
    return http.get('/api/memory/list', params)
  }
}
