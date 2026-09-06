import { http } from '@/utils/request'

export function bind(data) {
  return http.post('/api/agent-kb-bindings', data)
}

export function unbind(agentId, kbId) {
  return http.delete(`/api/agent-kb-bindings/${agentId}/${kbId}`)
}

export function listByAgent(agentId) {
  return http.get(`/api/agent-kb-bindings/by-agent/${agentId}`)
}

export function listByKB(kbId) {
  return http.get(`/api/agent-kb-bindings/by-kb/${kbId}`)
}

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
