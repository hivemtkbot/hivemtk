import { http } from '@/utils/request';

export function listBindings(params) {
  return http.get('/api/channel-agent-bindings', params)
}

export function listBindingsByAgent(agentId) {
  return http.get(`/api/channel-agent-bindings/by-agent/${agentId}`)
}

export function createBinding(data) {
  return http.post('/api/channel-agent-bindings', data)
}

export function updateBinding(id, data) {
  return http.put(`/api/channel-agent-bindings/${id}`, data)
}

export function deleteBinding(id) {
  return http.delete(`/api/channel-agent-bindings/${id}`)
}
