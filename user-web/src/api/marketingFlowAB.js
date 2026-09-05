import { http } from '@/utils/request'

export const createMarketingFlowWithAB = (data) => {
  const { nodes, edges, ...rest } = data
  const enrichedNodes = nodes.map((n) => {
    if (n.type === 'abtest' && !n.data.experiment_id) {
      return { ...n, data: { ...n.data, experiment_id: 'auto_create' } }
    }
    return n
  });
  return http.post('/api/marketing-flows', { ...rest, nodes: enrichedNodes, edges })
};

export const listFlows = (params) => http.get('/api/marketing-flows', params);
export const getFlow = (id) => http.get(`/api/marketing-flows/${id}`)
export const updateFlow = (id, data) => http.put(`/api/marketing-flows/${id}`, data)
export const activateFlow = (id) => http.post(`/api/marketing-flows/${id}/activate`, {})
export const pauseFlow = (id) => http.post(`/api/marketing-flows/${id}/pause`, {})
export const getFlowStats = (id) => http.get(`/api/marketing-flows/${id}/stats`)

export const syncFlowABResults = (id) =>
  http.post(`/api/marketing-flows/${id}/sync-ab-results`, {});
