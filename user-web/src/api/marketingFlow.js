import { http } from '@/utils/request';

// 营销流程 - 匹配后端 /api/marketing-flows/* 路径
export function getMarketingFlowList(params) {
  return request({ url: '/api/marketing-flows', method: 'get', params })
}
export function getMarketingFlow(id) {
  return http.get(`/api/marketing-flows/${id}`)
}
export function createMarketingFlow(data) {
  return request({ url: '/api/marketing-flows', method: 'post', data })
}
export function updateMarketingFlow(id, data) {
  return request({ url: `/api/marketing-flows/${id}`, method: 'put', data })
}
export function deleteMarketingFlow(id) {
  return http.delete(`/api/marketing-flows/${id}`)
}
export function activateFlow(id) {
  return http.post(`/api/marketing-flows/${id}/activate`)
}
export function pauseMarketingFlow(id) {
  return http.post(`/api/marketing-flows/${id}/pause`)
}
export function stopMarketingFlow(id) {
  return http.post(`/api/marketing-flows/${id}/stop`)
}
export function getFlowExecutions(id) {
  return http.get(`/api/marketing-flows/${id}/executions`)
}
export function getFlowStats(id) {
  return http.get(`/api/marketing-flows/${id}/stats`)
}

// 兼容旧接口
export function getFlows(params) {
  return getMarketingFlowList(params)
}
export function createFlow(data) {
  return createMarketingFlow(data)
}
export function updateFlow(id, data) {
  return updateMarketingFlow(id, data)
}
export function deleteFlow(id) {
  return deleteMarketingFlow(id)
}
export function runFlow(id) {
  return activateFlow(id)
}
export function toggleFlow(id, enabled) {
  return enabled ? activateFlow(id) : pauseMarketingFlow(id)
}
export function getFlowLogs(id) {
  return getFlowExecutions(id)
}
