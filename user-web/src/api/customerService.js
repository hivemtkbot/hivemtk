import { http } from '@/utils/request';

export function createAgent(data) {
  return http.post('/api/agents', data)
}
export function getAgentStatus(id) {
  return http.get(`/api/agents/${id}`)
}
export function getOnlineAgents() {
  return http.get('/api/agents/online')
}
export function listAllAgents() {
  return http.get('/api/agents/all')
}
export function updateAgentStatus(id, data) {
  return http.put(`/api/agents/${id}/status`, data)
}
export function goOnline(id) {
  return http.post(`/api/agents/${id}/online`)
}
export function goOffline(id) {
  return http.post(`/api/agents/${id}/offline`)
}
export function getAgentSessions(id) {
  return http.get(`/api/agents/${id}/sessions`)
}
export function getMyAgent() {
  return http.get('/api/agents/me')
}

export function getQuickReplies(params) {
  return http.get('/api/quick-replies', params)
}
export function getQuickReplyCategories() {
  return http.get('/api/quick-replies/categories')
}
export function createQuickReply(data) {
  return http.post('/api/quick-replies', data)
}
export function updateQuickReply(id, data) {
  return http.put(`/api/quick-replies/${id}`, data)
}
export function deleteQuickReply(id) {
  return http.delete(`/api/quick-replies/${id}`)
}

export function getSessionTags() {
  return http.get('/api/session-tags')
}
export function createSessionTag(data) {
  return http.post('/api/session-tags', data)
}
export function updateSessionTag(id, data) {
  return http.put(`/api/session-tags/${id}`, data)
}
export function deleteSessionTag(id) {
  return http.delete(`/api/session-tags/${id}`)
}

export function getAISuggestions(sessionId) {
  return http.get(`/api/ai-suggestions/${sessionId}`)
}
export function useAISuggestion(id) {
  return http.post(`/api/ai-suggestions/${id}/use`)
}

export function tagSession(sessionId, data) {
  return http.post(`/api/customer-sessions/${sessionId}/tags`, data)
}
