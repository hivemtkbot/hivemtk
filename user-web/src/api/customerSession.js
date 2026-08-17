import { http } from '@/utils/request';

// 客户会话管理 - 匹配后端 customer-sessions 路由 (service_routes.go)
// 注意：后端前缀为 /api/customer-sessions（连字符），旧版 /api/customer/session 不存在
export function getSessions(params) {
  return http.get('/api/customer-sessions', params)
}
export function getSessionMessages(id, params) {
  return http.get(`/api/customer-sessions/${id}/messages`, params)
}
// 坐席/AI 回复：sender_type 必填，后端据此落库并实时推送给访客 WebSocket
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

// 方向10：坐席实时聊天看板 - AI/人工接管与切换
//
// 后端对应路由（service_routes.go）：
//   POST /api/customer-sessions/:id/takeover        坐席接管
//   POST /api/customer-sessions/:id/release         释放回 AI
//   POST /api/customer-sessions/:id/switch-handler  统一切换
// 鉴权：依赖 JWT 中间件；agent_id 由后端从 ctx.user_id 派生，禁止前端伪造

/**
 * 坐席接管 AI 会话
 * @param {number|string} id 会话数字主键 ID（后端 :id 路由参数）
 * @param {string} [reason] 接管原因
 */
export function takeoverSession(id, reason = '') {
  return http.post(`/api/customer-sessions/${id}/takeover`, { reason })
}

/**
 * 坐席释放会话回 AI
 * @param {number|string} id 会话 ID
 */
export function releaseSession(id) {
  return http.post(`/api/customer-sessions/${id}/release`)
}

/**
 * 统一 AI/人工切换
 * @param {number|string} id 会话 ID
 * @param {'ai'|'human'} handlerType 目标处理方
 * @param {string} [reason] 切换原因
 */
export function switchSessionHandler(id, handlerType, reason = '') {
  return http.post(`/api/customer-sessions/${id}/switch-handler`, { handler_type: handlerType, reason })
}

/**
 * 拉黑当前会话对应的访客（user_id 维度）
 * @param {number|string} id 会话 ID
 * @param {string} [reason] 拉黑原因
 * @param {number} [ttlHours] 0 = 永久
 */
export function blacklistSession(id, reason = '', ttlHours = 0) {
  return http.post(`/api/customer-sessions/${id}/blacklist`, { reason, ttl_hours: ttlHours })
}

/**
 * 解除拉黑
 * @param {string} userId 访客 user_id
 * @param {string} platform 平台（web / douyin / ... ）
 */
export function unblacklistUser(userId, platform = 'web') {
  return http.post('/api/customer-sessions/blacklist/remove', { user_id: userId, platform })
}

/**
 * 查询访客是否在黑名单
 * @param {string} userId 访客 user_id
 * @param {string} platform 平台
 */
export function checkBlacklist(userId, platform = 'web') {
  return http.get('/api/customer-sessions/blacklist/check', { params: { user_id: userId, platform } })
}

/**
 * 黑名单分页列表
 * @param {object} params { page, page_size }
 */
export function listBlacklist(params = {}) {
  return http.get('/api/customer-sessions/blacklist', params)
}

// 客户 360° 画像（用于右栏渲染客户信息 + SOP 阶段）
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

