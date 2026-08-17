import { http } from '@/utils/request';

// 获取素材列表
export function getMaterialList(params = {}) {
  return http.get('/api/material/list', params)
}

// 上传素材
export function uploadMaterial(data) {
  return http.upload('/api/material/upload', data)
}

// 删除素材
export function deleteMaterial(id) {
  return http.delete(`/api/material/${id}`)
}

// 获取素材分类列表
export function getMaterialCategories() {
  return http.get('/api/material/categories')
}

// 创建素材分类
export function createMaterialCategory(data) {
  return http.post('/api/material/categories', data)
}

// 更新素材分类
export function updateMaterialCategory(id, data) {
  return http.put(`/api/material/categories/${id}`, data)
}

// 删除素材分类
export function deleteMaterialCategory(id) {
  return http.delete(`/api/material/categories/${id}`)
}

// 获取素材选择器数据
export function getMaterialSelector(params = {}) {
  return http.get('/api/material/selector', params)
}

// 更新素材使用次数
export function updateMaterialUsage(id) {
  return http.post(`/api/material/${id}/usage`)
}

// 获取素材统计信息
export function getMaterialStats() {
  return http.get('/api/material/stats')
}