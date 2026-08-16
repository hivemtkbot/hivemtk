import { http } from '@/utils/request';

// 获取素材列表
export function getMaterialList(params = {}) {
  return request({
    url: '/api/material/list',
    method: 'get',
    params
  })
}

// 上传素材
export function uploadMaterial(data) {
  return request({
    url: '/api/material/upload',
    method: 'post',
    data,
    headers: {
      'Content-Type': 'multipart/form-data'
    }
  })
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
  return request({
    url: '/api/material/categories',
    method: 'post',
    data
  })
}

// 更新素材分类
export function updateMaterialCategory(id, data) {
  return request({
    url: `/api/material/categories/${id}`,
    method: 'put',
    data
  })
}

// 删除素材分类
export function deleteMaterialCategory(id) {
  return http.delete(`/api/material/categories/${id}`)
}

// 获取素材选择器数据
export function getMaterialSelector(params = {}) {
  return request({
    url: '/api/material/selector',
    method: 'get',
    params
  })
}

// 更新素材使用次数
export function updateMaterialUsage(id) {
  return http.post(`/api/material/${id}/usage`)
}

// 获取素材统计信息
export function getMaterialStats() {
  return http.get('/api/material/stats')
}