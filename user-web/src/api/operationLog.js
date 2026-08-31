import { http } from '@/utils/request';

export function getOperationLogs(params) {
  return http.get('/api/operation-logs', params)
}
export function getOperationLogDetail(id) {
  return http.get(`/api/operation-logs/${id}`)
}
export function getOperationLogStatistics(params) {
  return http.get('/api/operation-logs/statistics', params)
}
export function exportOperationLogs(params) {
  // R42: responseType 必须走第 3 参 axios config——写进第 2 参会被当成 query 参数，
  //  CSV 字符串进响应拦截器报"非 JSON 响应"，导出必败
  return http.get('/api/operation-logs/export', params, { responseType: 'blob' })
}
export function deleteOperationLogs(ids) {
  return http.delete('/api/operation-logs', { data: { ids } })
}
export function cleanOperationLogs(beforeDate) {
  return http.post('/api/operation-logs/clean', { beforeDate })
}
