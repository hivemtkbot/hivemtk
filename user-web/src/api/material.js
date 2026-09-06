import { http } from '@/utils/request';

export function getMaterialList(params = {}) {
  return http.get('/api/material/list', params)
}

export function uploadMaterial(data) {
  return http.upload('/api/material/upload', data)
}

export function deleteMaterial(id) {
  return http.delete(`/api/material/${id}`)
}

export function getMaterialCategories() {
  return http.get('/api/material/categories')
}

export function createMaterialCategory(data) {
  return http.post('/api/material/categories', data)
}

export function updateMaterialCategory(id, data) {
  return http.put(`/api/material/categories/${id}`, data)
}

export function deleteMaterialCategory(id) {
  return http.delete(`/api/material/categories/${id}`)
}

export function getMaterialSelector(params = {}) {
  return http.get('/api/material/selector', params)
}

export function updateMaterialUsage(id) {
  return http.post(`/api/material/${id}/usage`)
}

export function getMaterialStats() {
  return http.get('/api/material/stats')
}