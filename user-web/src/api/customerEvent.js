import request from '@/utils/request'

// 客户事件追踪 - 匹配后端 events 路由
export function trackEvent(data) {
  return request({ url: '/api/events/track', method: 'post', data })
}
export function trackPageView(data) {
  return request({ url: '/api/events/pageview', method: 'post', data })
}
export function trackClick(data) {
  return request({ url: '/api/events/click', method: 'post', data })
}
export function trackPurchase(data) {
  return request({ url: '/api/events/purchase', method: 'post', data })
}
export function trackSignup(data) {
  return request({ url: '/api/events/signup', method: 'post', data })
}
export function trackLogin(data) {
  return request({ url: '/api/events/login', method: 'post', data })
}
export function trackAddToCart(data) {
  return request({ url: '/api/events/add-to-cart', method: 'post', data })
}
export function getCustomerEventHistory(customerId, params) {
  return request({ url: `/api/events/customer/${customerId}`, method: 'get', params })
}
export function getEventStats(params) {
  return request({ url: '/api/events/stats', method: 'get', params })
}
// 历史命名导出,保留兼容
export function getCustomerEvents(params) {
  return getEventStats(params)
}
export function createEvent(data) {
  return trackEvent(data)
}
export function getEventDetail(id) {
  return getCustomerEventHistory(String(id))
}
export function deleteEvent(id) {
  return request({ url: `/api/events/customer/${id}`, method: 'delete' })
}
