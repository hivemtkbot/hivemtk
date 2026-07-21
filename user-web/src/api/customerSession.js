import request from '@/utils/request'

// 客户会话管理 - 匹配后端 customer-sessions 路由 (service_routes.go)
// 注意：后端前缀为 /api/customer-sessions（连字符），旧版 /api/customer/session 不存在
export function getSessions(params) {
  return request({ url: '/api/customer-sessions', method: 'get', params })
}
export function getSessionMessages(id, params) {
  return request({ url: `/api/customer-sessions/${id}/messages`, method: 'get', params })
}
// 坐席/AI 回复：sender_type 必填，后端据此落库并实时推送给访客 WebSocket
export function sendMessage(data) {
  return request({
    url: `/api/customer-sessions/${data.sessionId}/messages`,
    method: 'post',
    data: {
      content: data.content,
      sender_type: data.sender_type || 'agent',
      sender_name: data.sender_name || '客服',
      sender_id: data.sender_id || ''
    }
  })
}
export function createSession(data) {
  return request({ url: '/api/customer-sessions', method: 'post', data })
}
export function closeSession(id) {
  return request({ url: `/api/customer-sessions/${id}/close`, method: 'post' })
}
export function transferSession(id, data) {
  return request({ url: `/api/customer-sessions/${id}/transfer`, method: 'post', data })
}
