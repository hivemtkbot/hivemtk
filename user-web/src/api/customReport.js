import { http } from '@/utils/request';

// 自定义报表 - 匹配后端 /api/custom-reports/* 路径
export function getReportList(params) {
  return request({ url: '/api/custom-reports', method: 'get', params })
}
export function getReport(id) {
  return http.get(`/api/custom-reports/${id}`)
}
export function createReport(data) {
  return request({ url: '/api/custom-reports', method: 'post', data })
}
export function updateReport(id, data) {
  return request({ url: `/api/custom-reports/${id}`, method: 'put', data })
}
export function deleteReport(id) {
  return http.delete(`/api/custom-reports/${id}`)
}
export function getPublicTemplates() {
  return http.get('/api/custom-reports/templates')
}
export function useReportTemplate(id) {
  return http.post(`/api/custom-reports/templates/${id}/use`)
}
export function queryReportData(id, params) {
  return request({ url: `/api/custom-reports/${id}/data`, method: 'get', params })
}

// 兼容旧接口
export function getCustomReports(params) {
  return getReportList(params)
}
export function createCustomReport(data) {
  return createReport(data)
}
export function updateCustomReport(id, data) {
  return updateReport(id, data)
}
export function deleteCustomReport(id) {
  return deleteReport(id)
}
export function exportCustomReport(id) {
  return http.get(`/api/custom-reports/${id}/data?format=export`)
}
export function runCustomReport(id) {
  return http.get(`/api/custom-reports/${id}/data?format=run`)
}
