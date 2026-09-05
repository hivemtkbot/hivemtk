import { http } from '@/utils/request';

export function createGlossary(data) {
  return http.post('/api/glossaries', data)
}

export function listGlossaries(params) {
  return http.get('/api/glossaries', params)
}

export function getGlossary(termId) {
  return http.get(`/api/glossaries/${termId}`)
}

export function updateGlossary(termId, data) {
  return http.put(`/api/glossaries/${termId}`, data)
}

export function deleteGlossary(termId) {
  return http.delete(`/api/glossaries/${termId}`)
}

export function validateGlossary(data) {
  return http.post('/api/glossaries/validate', data)
}

export default {
  createGlossary,
  listGlossaries,
  getGlossary,
  updateGlossary,
  deleteGlossary,
  validateGlossary
}
