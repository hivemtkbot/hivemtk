import request from '@/utils/request'

// 批量操作 - 匹配后端 /api/batch/* 路径
export function batchImportFile(data) {
  return request({ url: '/api/batch/import', method: 'post', data })
}
export function downloadBatchTemplate() {
  return request({ url: '/api/batch/template', method: 'get' })
}
export function batchExport(data) {
  return request({ url: '/api/batch/export', method: 'post', data })
}
export function batchDelete(data) {
  return request({ url: '/api/batch/delete', method: 'post', data })
}
export function batchUpdate(data) {
  return request({ url: '/api/batch/update', method: 'post', data })
}

// 兼容旧接口
export function getBatchTools() {
  return request({ url: '/api/batch/tools', method: 'get' })
}
export function runBatch(data) {
  return batchImportFile(data)
}
export function getBatchHistories(params) {
  return request({ url: '/api/batch/histories', method: 'get', params })
}
export function cancelBatch(id) {
  return request({ url: `/api/batch/histories/${id}/cancel`, method: 'post' })
}
export function getBatchDetail(id) {
  return request({ url: `/api/batch/histories/${id}`, method: 'get' })
}
export function previewBatch(data) {
  return request({ url: '/api/batch/preview', method: 'post', data })
}
