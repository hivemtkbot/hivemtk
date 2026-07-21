import request from '@/utils/request'

// 客户360视图 - 复用现有 customer API
export function getCustomerList(params) {
  return request({ url: '/api/customer/list', method: 'get', params })
}
export function getCustomer360Detail(id) {
  return request({ url: `/api/customer/360/${id}`, method: 'get' })
}
export function addCustomerTag(id, tag) {
  return request({ url: `/api/customer/${id}/tags`, method: 'post', data: { tag } })
}
export function removeCustomerTag(id, tag) {
  return request({ url: `/api/customer/${id}/tags/${tag}`, method: 'delete' })
}
export function updateCustomer(id, data) {
  return request({ url: `/api/customer/${id}`, method: 'put', data })
}
export function getCustomerDetail(id) {
  return request({ url: `/api/customer/${id}`, method: 'get' })
}
export function getCustomerBehaviors(id) {
  return request({ url: `/api/customer/${id}/behaviors`, method: 'get' })
}
export function getCustomerOrders(id) {
  return request({ url: `/api/customer/${id}/orders`, method: 'get' })
}
export function getCustomerCommunications(id) {
  return request({ url: `/api/customer/${id}/communications`, method: 'get' })
}
