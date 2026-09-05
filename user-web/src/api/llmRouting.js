import { http } from '@/utils/request'

export const LlmRoutingApi = {
  getModelList: (params) => {
    return http.get('/api/llm/models', params)
  },

  getModelDetail: (name) => {
    return http.get(`/api/llm/models/${encodeURIComponent(name)}`)
  },

  createModel: (data) => {
    return http.post('/api/llm/models', data)
  },

  updateModel: (name, data) => {
    return http.put(`/api/llm/models/${encodeURIComponent(name)}`, data)
  },

  deleteModel: (name) => {
    return http.delete(`/api/llm/models/${encodeURIComponent(name)}`)
  },

  updateModelStatus: (name, status) => {
    return http.put(`/api/llm/models/${encodeURIComponent(name)}`, { enabled: status })
  },

  testModel: (name, body = {}) => {
    return http.post(`/api/llm/models/${encodeURIComponent(name)}/test`, body)
  },

  getSceneRouting: () => {
    return http.get('/api/llm/scene-routing')
  },

  getScenarios: () => {
    return http.get('/api/llm/scenarios')
  },

  saveSceneRouting: (data) => {
    const payload = Array.isArray(data) ? { routes: data } : data
    return http.put('/api/llm/scene-routing', payload)
  },

  getFallbackStrategy: () => {
    return http.get('/api/llm/fallback')
  },

  getAuditHistory: (scenario, limit = 50) => {
    const params = { limit }
    if (scenario) params.scenario = scenario
    return http.get('/api/llm/audit', params)
  },

  getStats: () => {
    return http.get('/api/llm/stats')
  },

  getUsage: (window = 'all') => {
    return http.get('/api/llm/usage', { window })
  },

  getCostStats: (window = 'month') => {
    return http.get('/api/llm/cost-stats', { window })
  },

  getHealth: () => {
    return http.get('/api/llm/health')
  },

  getModelTypeStats: (window = 'month') => {
    return http.get('/api/llm/model-type-stats', { window })
  },

  getEgressAudit: (params = {}) => {
    return http.get('/api/llm/egress-audit', params)
  },

  getEgressAlerts: (params = {}) => {
    return http.get('/api/llm/egress-alerts', params)
  }
};
