import { http } from '@/utils/request';

// ============================================================================
// 渠道账号绑定智能体 API
// ----------------------------------------------------------------------------
// 对应后端 /api/channel-agent-bindings 路由
// ============================================================================

// 按渠道账号查询绑定列表
export function listBindings(params) {
  return http.get('/api/channel-agent-bindings', params)
}

// 反查智能体被哪些渠道使用
export function listBindingsByAgent(agentId) {
  return http.get(`/api/channel-agent-bindings/by-agent/${agentId}`)
}

// 创建绑定
export function createBinding(data) {
  return http.post('/api/channel-agent-bindings', data)
}

// 更新绑定
export function updateBinding(id, data) {
  return http.put(`/api/channel-agent-bindings/${id}`, data)
}

// 删除绑定
export function deleteBinding(id) {
  return http.delete(`/api/channel-agent-bindings/${id}`)
}
