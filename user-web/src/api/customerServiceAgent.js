import request from '@/utils/request'

// ============================================================================
// 客服座席挂载智能体 API
// ----------------------------------------------------------------------------
// 对应后端 /api/customer-service-agents 路由
// ============================================================================

// 按座席查询挂载列表
export function listMounts(params) {
  return request({
    url: '/api/customer-service-agents',
    method: 'get',
    params
  })
}

// 反查智能体被哪些客服使用
export function listMountsByAIAgent(aiAgentId) {
  return request({
    url: `/api/customer-service-agents/by-ai-agent/${aiAgentId}`,
    method: 'get'
  })
}

// 创建挂载
export function createMount(data) {
  return request({
    url: '/api/customer-service-agents',
    method: 'post',
    data
  })
}

// 更新挂载
export function updateMount(id, data) {
  return request({
    url: `/api/customer-service-agents/${id}`,
    method: 'put',
    data
  })
}

// 删除挂载
export function deleteMount(id) {
  return request({
    url: `/api/customer-service-agents/${id}`,
    method: 'delete'
  })
}

// 按用户ID查询挂载（团队成员即座席）
export function listMountsByUser(userId) {
  return request({
    url: `/api/customer-service-agents/by-user/${userId}`,
    method: 'get'
  })
}

// 按用户ID创建挂载（自动创建座席状态）
export function createMountByUser(userId, data) {
  return request({
    url: `/api/customer-service-agents/by-user/${userId}`,
    method: 'post',
    data
  })
}
