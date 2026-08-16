import { http } from '@/utils/request';

// 客户360视图 - 复用现有 customer API
export function getCustomerList(params) {
  return request({ url: '/api/customer/list', method: 'get', params })
}
export function getCustomer360Detail(id) {
  return http.get(`/api/customer/360/${id}`)
}
export function addCustomerTag(id, tag) {
  return request({ url: `/api/customer/${id}/tags`, method: 'post', data: { tag } })
}
export function removeCustomerTag(id, tag) {
  return http.delete(`/api/customer/${id}/tags/${tag}`)
}
export function updateCustomer(id, data) {
  return request({ url: `/api/customer/${id}`, method: 'put', data })
}
export function getCustomerDetail(id) {
  return http.get(`/api/customer/${id}`)
}
export function getCustomerBehaviors(id) {
  return http.get(`/api/customer/${id}/behaviors`)
}
export function getCustomerCommunications(id) {
  return http.get(`/api/customer/${id}/communications`)
}
