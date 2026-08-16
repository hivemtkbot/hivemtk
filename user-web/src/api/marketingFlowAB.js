import { http } from '@/utils/request'

/**
 * 营销流程 A/B 节点（USR-RC-07）
 * marketingFlow 节点可绑定 A/B 实验
 */

// 创建带 A/B 节点的流程
export const createMarketingFlowWithAB = (data) => {
  const { nodes, edges, ...rest } = data
  // 注入 A/B 实验节点
  const enrichedNodes = nodes.map((n) => {
    if (n.type === 'abtest' && !n.data.experiment_id) {
      return { ...n, data: { ...n.data, experiment_id: 'auto_create' } }
    }
    return n
  })
  return http.post('/api/marketing-flows', { ...rest, nodes: enrichedNodes, edges })
}

// 列表
export const listFlows = (params) => http.get('/api/marketing-flows', params)
export const getFlow = (id) => http.get(`/api/marketing-flows/${id}`)
export const updateFlow = (id, data) => http.put(`/api/marketing-flows/${id}`, data)
export const activateFlow = (id) => http.post(`/api/marketing-flows/${id}/activate`, {})
export const pauseFlow = (id) => http.post(`/api/marketing-flows/${id}/pause`, {})
export const getFlowStats = (id) => http.get(`/api/marketing-flows/${id}/stats`)

// 实验效果回流
export const syncFlowABResults = (id) =>
  http.post(`/api/marketing-flows/${id}/sync-ab-results`, {})
