import { http } from '@/utils/request'

// ============================================================================
// 智能体 ↔ 知识库 多对多绑定 API
// ----------------------------------------------------------------------------
// 后端路由前缀：/api/agent-kb-bindings
// 关系：一个智能体可挂载多个知识库（多类型），一个知识库可被多个智能体使用
// ============================================================================

// 绑定一个或多个知识库到智能体
//   data = { agent_id, kb_ids: [1,2,3] } 或 { agent_id, kb_type: 'rag' }
export function bind(data) {
  return http.post('/api/agent-kb-bindings', data)
}

// 解除智能体与某个知识库的绑定
export function unbind(agentId, kbId) {
  return http.delete(`/api/agent-kb-bindings/${agentId}/${kbId}`)
}

// 查询某智能体已挂载的知识库列表（含 KB 详情）
export function listByAgent(agentId) {
  return http.get(`/api/agent-kb-bindings/by-agent/${agentId}`)
}

// 反查某知识库被哪些智能体使用
export function listByKB(kbId) {
  return http.get(`/api/agent-kb-bindings/by-kb/${kbId}`)
}

// 全量替换智能体的知识库挂载列表（编辑页提交场景）
export function replaceAgentKBs(agentId, kbIds) {
  return http.put(`/api/agent-kb-bindings/by-agent/${agentId}`, { kb_ids: kbIds })
}

export const agentKBBindingAPI = {
  bind,
  unbind,
  listByAgent,
  listByKB,
  replaceAgentKBs
}

export default agentKBBindingAPI
