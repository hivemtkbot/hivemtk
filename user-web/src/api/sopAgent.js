import { http } from '@/utils/request'

export const sopApi = {
  list(params) {
    return http.get('/api/sop', params)
  },

  create(data) {
    return http.post('/api/sop', data)
  },

  getStats() {
    return http.get('/api/sop/stats')
  },

  matchByIntent(params) {
    return http.get('/api/sop/match', params)
  },

  listExecutions(params) {
    return http.get('/api/sop/executions', params)
  },

  getExecution(id) {
    return http.get(`/api/sop/executions/${id}`)
  },

  pauseExecution(id) {
    return http.post(`/api/sop/executions/${id}/pause`)
  },

  resumeExecution(id) {
    return http.post(`/api/sop/executions/${id}/resume`)
  },

  cancelExecution(id) {
    return http.post(`/api/sop/executions/${id}/cancel`)
  },

  get(id) {
    return http.get(`/api/sop/${id}`)
  },

  update(id, data) {
    return http.put(`/api/sop/${id}`, data)
  },

  remove(id) {
    return http.delete(`/api/sop/${id}`)
  },

  activate(id) {
    return http.post(`/api/sop/${id}/activate`)
  },

  deactivate(id) {
    return http.post(`/api/sop/${id}/deactivate`)
  },

  execute(data) {
    return http.post('/api/sop/execute', data)
  },

  step(data) {
    return http.post('/api/sop/step', data)
  }
};
