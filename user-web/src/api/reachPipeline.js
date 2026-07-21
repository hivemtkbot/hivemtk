import { http } from '@/utils/request'

/**
 * 触达 Pipeline 框架 API
 * 匹配后端 /api/reach/* 路径
 * 后端控制器: internal/controller/reach_pipeline_controller.go
 */

/**
 * Pipeline 列表查询参数
 * @typedef {Object} PipelineListParams
 * @property {number} [page]
 * @property {number} [page_size]
 * @property {string} [channel]
 * @property {string} [status]
 */

/**
 * 创建/更新 Pipeline 请求体
 * @typedef {Object} PipelinePayload
 */

/**
 * 任务列表查询参数
 * @typedef {Object} JobListParams
 * @property {number} [page]
 * @property {number} [page_size]
 * @property {string} [channel]
 * @property {string} [state]
 */

/**
 * 入队任务请求体
 * @typedef {Object} EnqueueJobPayload
 */

export const reachPipelineApi = {
  // ===== Pipeline =====
  getPipelines(params) {
    return http.get('/api/reach/pipelines', params)
  },
  createPipeline(data) {
    return http.post('/api/reach/pipelines', data)
  },
  getPipeline(id) {
    return http.get(`/api/reach/pipelines/${id}`)
  },
  updatePipeline(id, data) {
    return http.put(`/api/reach/pipelines/${id}`, data)
  },
  deletePipeline(id) {
    return http.delete(`/api/reach/pipelines/${id}`)
  },
  pausePipeline(id) {
    return http.post(`/api/reach/pipelines/${id}/pause`, {})
  },
  resumePipeline(id) {
    return http.post(`/api/reach/pipelines/${id}/resume`, {})
  },
  archivePipeline(id) {
    return http.post(`/api/reach/pipelines/${id}/archive`, {})
  },

  // ===== Job =====
  getJobs(params) {
    return http.get('/api/reach/jobs', params)
  },
  enqueueJob(data) {
    return http.post('/api/reach/jobs', data)
  },
  getJob(id) {
    return http.get(`/api/reach/jobs/${id}`)
  },
  cancelJob(id) {
    return http.post(`/api/reach/jobs/${id}/cancel`, {})
  },
  retryJob(id) {
    return http.post(`/api/reach/jobs/${id}/retry`, {})
  },
  executeJob(id) {
    return http.post(`/api/reach/jobs/${id}/execute`, {})
  },

  // ===== 统计 / 限流 =====
  getStats(params) {
    return http.get('/api/reach/stats', params)
  },
  // 重置限流：后端 ResetRateLimit 校验 channel query 参数非空，故必须传入 channel
  resetRateLimit(channel) {
    return http.post('/api/reach/rate-limit/reset', {}, { params: { channel } })
  }
}
