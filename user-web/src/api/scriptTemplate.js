import { http } from '@/utils/request';

export function getScriptTemplateList(params) {
  return http.get('/api/scripts', params)
}
export function getScriptTemplate(id) {
  return http.get(`/api/scripts/${id}`)
}
export function createScriptTemplate(data) {
  return http.post('/api/scripts', data)
}
export function updateScriptTemplate(id, data) {
  return http.put(`/api/scripts/${id}`, data)
}
export function deleteScriptTemplate(id) {
  return http.delete(`/api/scripts/${id}`)
}
export function getScriptCategories() {
  return http.get('/api/scripts/categories')
}
export function searchScriptTemplates(params) {
  return http.get('/api/scripts/search', params)
}
export function getPublicScriptTemplates() {
  return http.get('/api/scripts/public')
}
export function recommendScript(data) {
  return http.post('/api/scripts/recommend', data)
}

export function getScripts(params) {
  return getScriptTemplateList(params)
}
export function createScript(data) {
  return createScriptTemplate(data)
}
export function updateScript(id, data) {
  return updateScriptTemplate(id, data)
}
export function deleteScript(id) {
  return deleteScriptTemplate(id)
}
export function useScript(id) {
  return getScriptTemplate(id)
}
export function rateScript(id, rating) {
  return recommendScript({ id, rating })
}
