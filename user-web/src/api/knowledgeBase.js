import { http } from '@/utils/request'

export function listKBs(params) {
  return http.get('/api/knowledge-bases', params)
}

export function listByType(kbType, params = {}) {
  return http.get(`/api/knowledge-bases/by-type/${kbType}`, params)
}

export function getKB(id) {
  return http.get(`/api/knowledge-bases/${id}`)
}

export function createKB(data) {
  return http.post('/api/knowledge-bases', data)
}

export function updateKB(id, data) {
  return http.put(`/api/knowledge-bases/${id}`, data)
}

export function deleteKB(id) {
  return http.delete(`/api/knowledge-bases/${id}`)
}

export function getKBStats(id) {
  return http.get(`/api/knowledge-bases/${id}/stats`)
}

export const knowledgeBaseAPI = {
  listKBs,
  getKB,
  createKB,
  updateKB,
  deleteKB,
  listByType,
  getKBStats
}

export default knowledgeBaseAPI
