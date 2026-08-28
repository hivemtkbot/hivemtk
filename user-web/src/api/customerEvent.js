import { http } from '@/utils/request';

// 客户事件追踪 - 匹配后端 events 路由
export function trackEvent(data) {
  return http.post('/api/events/track', data)
}
export function trackPageView(data) {
  return http.post('/api/events/pageview', data)
}
export function trackClick(data) {
  return http.post('/api/events/click', data)
}
export function trackPurchase(data) {
  return http.post('/api/events/purchase', data)
}
export function trackSignup(data) {
  return http.post('/api/events/signup', data)
}
export function trackLogin(data) {
  return http.post('/api/events/login', data)
}
export function trackAddToCart(data) {
  return http.post('/api/events/add-to-cart', data)
}
export function getCustomerEventHistory(customerId, params) {
  // R5-D1 修复: id 可能含非 ASCII/特殊字符(如 conv:中文会话名), 必须编码否则产生非法请求行 → 404
  return http.get(`/api/events/customer/${encodeURIComponent(customerId)}`, params)
}
export function getEventStats(params) {
  return http.get('/api/events/stats', params)
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
  // R5-D1 修复: 同上, id 编码
  return http.delete(`/api/events/customer/${encodeURIComponent(id)}`)
}
