import request from '@/utils/request'

// ============================================================================
// AI 智能体管理 API
// ----------------------------------------------------------------------------
// 对应后端 /api/ai-agents 路由
// ============================================================================

// 智能体列表
export function listAgents(params) {
  return request({
    url: '/api/ai-agents',
    method: 'get',
    params
  })
}

// 启用的智能体列表（下拉选择用）
export function listEnabledAgents() {
  return request({
    url: '/api/ai-agents-enabled',
    method: 'get'
  })
}

// 智能体详情
export function getAgent(id) {
  return request({
    url: `/api/ai-agents/${id}`,
    method: 'get'
  })
}

// 创建智能体
export function createAgent(data) {
  return request({
    url: '/api/ai-agents',
    method: 'post',
    data
  })
}

// 更新智能体
export function updateAgent(id, data) {
  return request({
    url: `/api/ai-agents/${id}`,
    method: 'put',
    data
  })
}

// 删除智能体
export function deleteAgent(id) {
  return request({
    url: `/api/ai-agents/${id}`,
    method: 'delete'
  })
}

// 启用/禁用智能体
export function toggleAgent(id, status) {
  return request({
    url: `/api/ai-agents/${id}/toggle`,
    method: 'post',
    data: { status }
  })
}

// 测试智能体执行
export function testAgent(id, data) {
  return request({
    url: `/api/ai-agents/${id}/test`,
    method: 'post',
    data
  })
}

// 获取智能体执行上下文（调试用）
export function getAgentContext(id) {
  return request({
    url: `/api/ai-agents/${id}/context`,
    method: 'get'
  })
}
