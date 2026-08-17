import { http } from '@/utils/request';

// 仪表板大屏 - 匹配后端 /api/dashboards/* 路径
export function getScreenList(params) {
  return http.get('/api/dashboards', params)
}
export function getScreenByID(id) {
  return http.get(`/api/dashboards/${id}`)
}
export function createScreen(data) {
  return http.post('/api/dashboards', data)
}
export function updateScreen(id, data) {
  return http.put(`/api/dashboards/${id}`, data)
}
export function deleteScreen(id) {
  return http.delete(`/api/dashboards/${id}`)
}
export function publicViewScreen(code) {
  return http.get(`/api/dashboards/public/${code}`)
}

// 真实大屏数据接口(后端聚合) - 严禁使用模拟数据
export function getDashboardData() {
  return http.get('/api/dashboards/data')
}
export function getRealtimeActivities() {
  return http.get('/api/dashboards/activities')
}
export function getKPIs() {
  return http.get('/api/dashboards/data')
}
export function getTrends(params) {
  return http.get('/api/dashboards/data', params)
}
export function getChannels() {
  return http.get('/api/dashboards/data')
}
export function getRegions() {
  return http.get('/api/dashboards/data')
}
