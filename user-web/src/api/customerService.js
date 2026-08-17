import { http } from '@/utils/request';

// 客服子功能 (坐席状态 / 快捷回复 / 会话标签 / AI建议)
// 对应后端 controller/customer_session.go 的 4 个子 Controller
//   - AgentStatusController:  /api/agents
//   - QuickReplyController:   /api/quick-replies
//   - SessionTagController:   /api/session-tags
//   - AISuggestionController: /api/ai-suggestions

// ========== 坐席状态 ==========
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
// 当前登录用户对应的坐席身份（基于 JWT）
export function getMyAgent() {
  return http.get('/api/agents/me')
}

// ========== 快捷回复 ==========
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

// ========== 会话标签 ==========
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

// ========== AI 建议 ==========
export function getAISuggestions(sessionId) {
  return http.get(`/api/ai-suggestions/${sessionId}`)
}
export function useAISuggestion(id) {
  return http.post(`/api/ai-suggestions/${id}/use`)
}

// ========== 会话打标（复用后端 TagSession 路由） ==========
export function tagSession(sessionId, data) {
  return http.post(`/api/customer-sessions/${sessionId}/tags`, data)
}
