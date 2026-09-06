import { http } from '@/utils/request';

export function listMounts(params) {
  return http.get('/api/customer-service-agents', params)
}

export function listMountsByAIAgent(aiAgentId) {
  return http.get(`/api/customer-service-agents/by-ai-agent/${aiAgentId}`)
}

export function createMount(data) {
  return http.post('/api/customer-service-agents', data)
}

export function updateMount(id, data) {
  return http.put(`/api/customer-service-agents/${id}`, data)
}

export function deleteMount(id) {
  return http.delete(`/api/customer-service-agents/${id}`)
}

export function listMountsByUser(userId) {
  return http.get(`/api/customer-service-agents/by-user/${userId}`)
}

export function createMountByUser(userId, data) {
  return http.post(`/api/customer-service-agents/by-user/${userId}`, data)
}
