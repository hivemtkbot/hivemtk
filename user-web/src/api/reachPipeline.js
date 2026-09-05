import { http } from '@/utils/request'

export const reachPipelineApi = {
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

  getStats(params) {
    return http.get('/api/reach/stats', params)
  },
  resetRateLimit(channel) {
    return http.post('/api/reach/rate-limit/reset', {}, { params: { channel } })
  }
};
