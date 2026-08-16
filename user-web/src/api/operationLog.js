import { http } from '@/utils/request';

export function getOperationLogs(params) {
  return request({ url: '/api/operation-logs', method: 'get', params })
}
export function getOperationLogDetail(id) {
  return http.get(`/api/operation-logs/${id}`)
}
export function getOperationLogStatistics(params) {
  return request({ url: '/api/operation-logs/statistics', method: 'get', params })
}
export function exportOperationLogs(params) {
  return request({
    url: '/api/operation-logs/export',
    method: 'get',
    params,
    responseType: 'blob'
  })
}
export function deleteOperationLogs(ids) {
  return request({ url: '/api/operation-logs', method: 'delete', data: { ids } })
}
export function cleanOperationLogs(beforeDate) {
  return request({ url: '/api/operation-logs/clean', method: 'post', data: { beforeDate } })
}
