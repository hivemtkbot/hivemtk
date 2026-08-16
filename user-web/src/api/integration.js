import { http } from '@/utils/request';

// 集成管理 - 匹配后端 /api/integrations/* 路径
export function getIntegrationAccountList(params) {
  return request({ url: '/api/integrations', method: 'get', params })
}
export function getIntegrationAccount(id) {
  return http.get(`/api/integrations/${id}`)
}
export function createIntegrationAccount(data) {
  return request({ url: '/api/integrations', method: 'post', data })
}
export function updateIntegrationAccount(id, data) {
  return request({ url: `/api/integrations/${id}`, method: 'put', data })
}
export function deleteIntegrationAccount(id) {
  return http.delete(`/api/integrations/${id}`)
}
export function syncCustomers(id) {
  return http.post(`/api/integrations/${id}/sync-customers`)
}
export function syncProducts(id) {
  return http.post(`/api/integrations/${id}/sync-products`)
}
export function getSyncLogs(params) {
  return request({ url: '/api/integration/sync-logs', method: 'get', params })
}
export function getExternalCustomers() {
  return http.get('/api/integration/external-customers')
}
export function getExternalOrders() {
  return http.get('/api/integration/external-orders')
}
export function getExternalOrdersByCustomer(phone, name) {
  return request({ url: '/api/integration/external-orders-by-customer', method: 'get', params: { phone, name } })
}
export function getExternalProducts() {
  return http.get('/api/integration/external-products')
}

// 兼容旧接口
export function getIntegrations(params) {
  return getIntegrationAccountList(params)
}
export function createIntegration(data) {
  return createIntegrationAccount(data)
}
export function updateIntegration(id, data) {
  return updateIntegrationAccount(id, data)
}
export function deleteIntegration(id) {
  return deleteIntegrationAccount(id)
}
export function toggleIntegrationStatus(id, enabled) {
  return updateIntegrationAccount(id, { enabled })
}
export function testIntegration(id) {
  return http.post(`/api/integrations/${id}/test`)
}
export function getIntegrationStats() {
  return http.get('/api/integrations')
}
export function getIntegrationLogs(id) {
  return getSyncLogs({ integration_id: id })
}
