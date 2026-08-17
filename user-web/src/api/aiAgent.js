import { http } from '@/utils/request';

// ============================================================================
// AI 智能体管理 API
// ----------------------------------------------------------------------------
// 对应后端 /api/ai-agents 路由
// ============================================================================

// 智能体列表
export function listAgents(params) {
  return http.get('/api/ai-agents', params)
}

// 启用的智能体列表（下拉选择用）
export function listEnabledAgents() {
  return http.get('/api/ai-agents-enabled')
}

// 智能体详情
export function getAgent(id) {
  return http.get(`/api/ai-agents/${id}`)
}

// 创建智能体
export function createAgent(data) {
  return http.post('/api/ai-agents', data)
}

// 更新智能体
export function updateAgent(id, data) {
  return http.put(`/api/ai-agents/${id}`, data)
}

// 删除智能体
export function deleteAgent(id) {
  return http.delete(`/api/ai-agents/${id}`)
}

// 启用/禁用智能体
export function toggleAgent(id, status) {
  return http.post(`/api/ai-agents/${id}/toggle`, { status })
}

// 测试智能体执行
export function testAgent(id, data) {
  return http.post(`/api/ai-agents/${id}/test`, data)
}

// 获取智能体执行上下文（调试用）
export function getAgentContext(id) {
  return http.get(`/api/ai-agents/${id}/context`)
}
