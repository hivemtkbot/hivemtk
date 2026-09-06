import { http } from '@/utils/request';

export function batchImportFile(data) {
  return http.post('/api/batch/import', data)
}
export function downloadBatchTemplate() {
  return http.get('/api/batch/template')
}
export function batchExport(data) {
  return http.post('/api/batch/export', data)
}
export function batchDelete(data) {
  return http.post('/api/batch/delete', data)
}
export function batchUpdate(data) {
  return http.post('/api/batch/update', data)
}

export function getBatchTools() {
  return http.get('/api/batch/tools')
}
export function runBatch(data) {
  return batchImportFile(data)
}
export function getBatchHistories(params) {
  return http.get('/api/batch/histories', params)
}
export function cancelBatch(id) {
  return http.post(`/api/batch/histories/${id}/cancel`)
}
export function getBatchDetail(id) {
  return http.get(`/api/batch/histories/${id}`)
}
export function previewBatch(data) {
  return http.post('/api/batch/preview', data)
}
