import request from '@/utils/request'

// ============================================================================
// 渠道账号绑定智能体 API
// ----------------------------------------------------------------------------
// 对应后端 /api/channel-agent-bindings 路由
// ============================================================================

// 按渠道账号查询绑定列表
export function listBindings(params) {
  return request({
    url: '/api/channel-agent-bindings',
    method: 'get',
    params
  })
}

// 反查智能体被哪些渠道使用
export function listBindingsByAgent(agentId) {
  return request({
    url: `/api/channel-agent-bindings/by-agent/${agentId}`,
    method: 'get'
  })
}

// 创建绑定
export function createBinding(data) {
  return request({
    url: '/api/channel-agent-bindings',
    method: 'post',
    data
  })
}

// 更新绑定
export function updateBinding(id, data) {
  return request({
    url: `/api/channel-agent-bindings/${id}`,
    method: 'put',
    data
  })
}

// 删除绑定
export function deleteBinding(id) {
  return request({
    url: `/api/channel-agent-bindings/${id}`,
    method: 'delete'
  })
}
