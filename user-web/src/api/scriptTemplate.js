import { http } from '@/utils/request';

// 话术库 - 匹配后端 /api/scripts/* 路径
export function getScriptTemplateList(params) {
  return request({ url: '/api/scripts', method: 'get', params })
}
export function getScriptTemplate(id) {
  return http.get(`/api/scripts/${id}`)
}
export function createScriptTemplate(data) {
  return request({ url: '/api/scripts', method: 'post', data })
}
export function updateScriptTemplate(id, data) {
  return request({ url: `/api/scripts/${id}`, method: 'put', data })
}
export function deleteScriptTemplate(id) {
  return http.delete(`/api/scripts/${id}`)
}
export function getScriptCategories() {
  return http.get('/api/scripts/categories')
}
export function searchScriptTemplates(params) {
  return request({ url: '/api/scripts/search', method: 'get', params })
}
export function getPublicScriptTemplates() {
  return http.get('/api/scripts/public')
}
export function recommendScript(data) {
  return request({ url: '/api/scripts/recommend', method: 'post', data })
}

// 兼容旧接口
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
