import request from '@/utils/request'

// 集成管理 - 匹配后端 /api/integrations/* 路径
export function getIntegrationAccountList(params) {
  return request({ url: '/api/integrations', method: 'get', params })
}
export function getIntegrationAccount(id) {
  return request({ url: `/api/integrations/${id}`, method: 'get' })
}
export function createIntegrationAccount(data) {
  return request({ url: '/api/integrations', method: 'post', data })
}
export function updateIntegrationAccount(id, data) {
  return request({ url: `/api/integrations/${id}`, method: 'put', data })
}
export function deleteIntegrationAccount(id) {
  return request({ url: `/api/integrations/${id}`, method: 'delete' })
}
export function syncCustomers(id) {
  return request({ url: `/api/integrations/${id}/sync-customers`, method: 'post' })
}
export function syncOrders(id) {
  return request({ url: `/api/integrations/${id}/sync-orders`, method: 'post' })
}
export function syncProducts(id) {
  return request({ url: `/api/integrations/${id}/sync-products`, method: 'post' })
}
export function getSyncLogs(params) {
  return request({ url: '/api/integration/sync-logs', method: 'get', params })
}
export function getExternalCustomers() {
  return request({ url: '/api/integration/external-customers', method: 'get' })
}
export function getExternalOrders() {
  return request({ url: '/api/integration/external-orders', method: 'get' })
}
export function getExternalOrdersByCustomer(phone, name) {
  return request({ url: '/api/integration/external-orders-by-customer', method: 'get', params: { phone, name } })
}
export function getExternalProducts() {
  return request({ url: '/api/integration/external-products', method: 'get' })
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
  return request({ url: `/api/integrations/${id}/test`, method: 'post' })
}
export function getIntegrationStats() {
  return request({ url: '/api/integrations', method: 'get' })
}
export function getIntegrationLogs(id) {
  return getSyncLogs({ integration_id: id })
}
