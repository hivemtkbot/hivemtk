import request from '@/utils/request'

// 客服子功能 (坐席状态 / 快捷回复 / 会话标签 / AI建议)
// 对应后端 controller/customer_session.go 的 4 个子 Controller
//   - AgentStatusController:  /api/agents
//   - QuickReplyController:   /api/quick-replies
//   - SessionTagController:   /api/session-tags
//   - AISuggestionController: /api/ai-suggestions

// ========== 坐席状态 ==========
export function createAgent(data) {
  return request({ url: '/api/agents', method: 'post', data })
}
export function getAgentStatus(id) {
  return request({ url: `/api/agents/${id}`, method: 'get' })
}
export function getOnlineAgents() {
  return request({ url: '/api/agents/online', method: 'get' })
}
export function listAllAgents() {
  return request({ url: '/api/agents/all', method: 'get' })
}
export function updateAgentStatus(id, data) {
  return request({ url: `/api/agents/${id}/status`, method: 'put', data })
}
export function goOnline(id) {
  return request({ url: `/api/agents/${id}/online`, method: 'post' })
}
export function goOffline(id) {
  return request({ url: `/api/agents/${id}/offline`, method: 'post' })
}
export function getAgentSessions(id) {
  return request({ url: `/api/agents/${id}/sessions`, method: 'get' })
}
// 当前登录用户对应的坐席身份（基于 JWT）
export function getMyAgent() {
  return request({ url: '/api/agents/me', method: 'get' })
}

// ========== 快捷回复 ==========
export function getQuickReplies(params) {
  return request({ url: '/api/quick-replies', method: 'get', params })
}
export function getQuickReplyCategories() {
  return request({ url: '/api/quick-replies/categories', method: 'get' })
}
export function createQuickReply(data) {
  return request({ url: '/api/quick-replies', method: 'post', data })
}
export function updateQuickReply(id, data) {
  return request({ url: `/api/quick-replies/${id}`, method: 'put', data })
}
export function deleteQuickReply(id) {
  return request({ url: `/api/quick-replies/${id}`, method: 'delete' })
}

// ========== 会话标签 ==========
export function getSessionTags() {
  return request({ url: '/api/session-tags', method: 'get' })
}
export function createSessionTag(data) {
  return request({ url: '/api/session-tags', method: 'post', data })
}
export function updateSessionTag(id, data) {
  return request({ url: `/api/session-tags/${id}`, method: 'put', data })
}
export function deleteSessionTag(id) {
  return request({ url: `/api/session-tags/${id}`, method: 'delete' })
}

// ========== AI 建议 ==========
export function getAISuggestions(sessionId) {
  return request({ url: `/api/ai-suggestions/${sessionId}`, method: 'get' })
}
export function useAISuggestion(id) {
  return request({ url: `/api/ai-suggestions/${id}/use`, method: 'post' })
}

// ========== 会话打标（复用后端 TagSession 路由） ==========
export function tagSession(sessionId, data) {
  return request({ url: `/api/customer-sessions/${sessionId}/tags`, method: 'post', data })
}
