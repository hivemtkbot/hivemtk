import request from '@/utils/request'

export function getOperationLogs(params) {
  return request({ url: '/api/team/logs', method: 'get', params })
}
export function getOperationLogDetail(id) {
  return request({ url: `/api/team/logs/${id}`, method: 'get' })
}
export function exportOperationLogs(params) {
  return request({
    url: '/api/team/logs/export',
    method: 'get',
    params,
    responseType: 'blob'
  })
}
export function deleteOperationLogs(ids) {
  return request({ url: '/api/team/logs', method: 'delete', data: { ids } })
}
export function cleanOperationLogs(beforeDate) {
  return request({ url: '/api/team/logs/clean', method: 'post', data: { beforeDate } })
}
