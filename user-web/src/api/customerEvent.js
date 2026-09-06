import { http } from '@/utils/request';

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
  return http.get(`/api/events/customer/${encodeURIComponent(customerId)}`, params);
}
export function getEventStats(params) {
  return http.get('/api/events/stats', params)
}
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
  return http.delete(`/api/events/customer/${encodeURIComponent(id)}`);
}

export function getGlobalEvents(params) {
  return http.get('/api/customer-events/list', params)
}
