import { http } from '@/utils/request'

// SOP 智能体 API - 匹配后端 /api/sop/* 路径

/**
 * @typedef {Object} SopListParams
 * @property {number} [page]
 * @property {number} [page_size]
 * @property {string} [keyword]
 * @property {string} [status]
 */

/**
 * @typedef {Object} SopMatchParams
 * @property {string} intent
 */

/**
 * @typedef {Object} SopExecutionListParams
 */

/**
 * @typedef {Object} SopExecuteParams
 * @property {string|number} sop_id
 * @property {string} customer_id
 * @property {Object} [context]
 */

/**
 * @typedef {Object} SopStepParams
 * @property {string|number} execution_id
 * @property {string} action
 * @property {Object} [input]
 */

export const sopApi = {
  // SOP 列表（params: page, page_size, keyword, status）
  list(params) {
    return http.get('/api/sop', params)
  },

  // 创建 SOP
  create(data) {
    return http.post('/api/sop', data)
  },

  // 统计
  getStats() {
    return http.get('/api/sop/stats')
  },

  // 按意图匹配（params: intent）
  matchByIntent(params) {
    return http.get('/api/sop/match', params)
  },

  // 执行列表（params: page, page_size, customer_id, status）
  listExecutions(params) {
    return http.get('/api/sop/executions', params)
  },

  // 执行详情
  getExecution(id) {
    return http.get(`/api/sop/executions/${id}`)
  },

  // 暂停执行
  pauseExecution(id) {
    return http.post(`/api/sop/executions/${id}/pause`)
  },

  // 恢复执行
  resumeExecution(id) {
    return http.post(`/api/sop/executions/${id}/resume`)
  },

  // 取消执行
  cancelExecution(id) {
    return http.post(`/api/sop/executions/${id}/cancel`)
  },

  // SOP 详情
  get(id) {
    return http.get(`/api/sop/${id}`)
  },

  // 更新 SOP
  update(id, data) {
    return http.put(`/api/sop/${id}`, data)
  },

  // 删除 SOP
  remove(id) {
    return http.delete(`/api/sop/${id}`)
  },

  // 激活 SOP
  activate(id) {
    return http.post(`/api/sop/${id}/activate`)
  },

  // 停用 SOP
  deactivate(id) {
    return http.post(`/api/sop/${id}/deactivate`)
  },

  // 执行 SOP
  execute(data) {
    return http.post('/api/sop/execute', data)
  },

  // 单步推进
  step(data) {
    return http.post('/api/sop/step', data)
  }
}
