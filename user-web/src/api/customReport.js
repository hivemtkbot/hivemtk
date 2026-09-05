import { http } from '@/utils/request';

export function getReportList(params) {
  return http.get('/api/custom-reports', params)
}
export function getReport(id) {
  return http.get(`/api/custom-reports/${id}`)
}
export function createReport(data) {
  return http.post('/api/custom-reports', data)
}
export function updateReport(id, data) {
  return http.put(`/api/custom-reports/${id}`, data)
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
  return http.get(`/api/custom-reports/${id}/data`, params)
}

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
