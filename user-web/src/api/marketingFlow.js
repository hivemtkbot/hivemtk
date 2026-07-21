import request from '@/utils/request'

// 营销流程 - 匹配后端 /api/marketing-flows/* 路径
export function getMarketingFlowList(params) {
  return request({ url: '/api/marketing-flows', method: 'get', params })
}
export function getMarketingFlow(id) {
  return request({ url: `/api/marketing-flows/${id}`, method: 'get' })
}
export function createMarketingFlow(data) {
  return request({ url: '/api/marketing-flows', method: 'post', data })
}
export function updateMarketingFlow(id, data) {
  return request({ url: `/api/marketing-flows/${id}`, method: 'put', data })
}
export function deleteMarketingFlow(id) {
  return request({ url: `/api/marketing-flows/${id}`, method: 'delete' })
}
export function activateFlow(id) {
  return request({ url: `/api/marketing-flows/${id}/activate`, method: 'post' })
}
export function pauseMarketingFlow(id) {
  return request({ url: `/api/marketing-flows/${id}/pause`, method: 'post' })
}
export function stopMarketingFlow(id) {
  return request({ url: `/api/marketing-flows/${id}/stop`, method: 'post' })
}
export function getFlowExecutions(id) {
  return request({ url: `/api/marketing-flows/${id}/executions`, method: 'get' })
}
export function getFlowStats(id) {
  return request({ url: `/api/marketing-flows/${id}/stats`, method: 'get' })
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
