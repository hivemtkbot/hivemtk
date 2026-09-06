import { http } from '@/utils/request';

export function getSessions(params) {
  return http.get('/api/customer-sessions', params)
}
export function getSessionMessages(id, params) {
  return http.get(`/api/customer-sessions/${id}/messages`, params)
}
export function sendMessage(data) {
  return http.post(`/api/customer-sessions/${data.sessionId}/messages`, {
    content: data.content,
    sender_type: data.sender_type || 'agent',
    sender_name: data.sender_name || '客服',
    sender_id: String(data.sender_id || '')
  })
}
export function createSession(data) {
  return http.post('/api/customer-sessions', data)
}
export function closeSession(id) {
  return http.post(`/api/customer-sessions/${id}/close`)
}
export function transferSession(id, data) {
  return http.post(`/api/customer-sessions/${id}/transfer`, data)
}

export function takeoverSession(id, reason = '') {
  return http.post(`/api/customer-sessions/${id}/takeover`, { reason })
}

export function releaseSession(id) {
  return http.post(`/api/customer-sessions/${id}/release`)
}

export function switchSessionHandler(id, handlerType, reason = '') {
  return http.post(`/api/customer-sessions/${id}/switch-handler`, { handler_type: handlerType, reason })
}

export function blacklistSession(id, reason = '', ttlHours = 0) {
  return http.post(`/api/customer-sessions/${id}/blacklist`, { reason, ttl_hours: ttlHours })
}

export function unblacklistUser(userId, platform = 'web') {
  return http.post('/api/customer-sessions/blacklist/remove', { user_id: userId, platform })
}

export function checkBlacklist(userId, platform = 'web') {
  return http.get('/api/customer-sessions/blacklist/check', { params: { user_id: userId, platform } })
}

export function listBlacklist(params = {}) {
  return http.get('/api/customer-sessions/blacklist', params)
}

export function getCustomer360(id) {
  return http.get(`/api/customer-360?user_id=${id}`)
}
export function getCustomerBasic(id) {
  return http.get(`/api/customer-360/basic?user_id=${id}`)
}
export function getCustomerStats(id) {
  return http.get(`/api/customer-360/stats?user_id=${id}`)
}
export function getCustomerSessions(id) {
  return http.get(`/api/customer-360/sessions?user_id=${id}`)
}
export function getCustomerTags(id) {
  return http.get(`/api/customer-360/tags?user_id=${id}`)
}

