import request from '@/utils/request'

// 仪表板大屏 - 匹配后端 /api/dashboards/* 路径
export function getScreenList(params) {
  return request({ url: '/api/dashboards', method: 'get', params })
}
export function getScreenByID(id) {
  return request({ url: `/api/dashboards/${id}`, method: 'get' })
}
export function createScreen(data) {
  return request({ url: '/api/dashboards', method: 'post', data })
}
export function updateScreen(id, data) {
  return request({ url: `/api/dashboards/${id}`, method: 'put', data })
}
export function deleteScreen(id) {
  return request({ url: `/api/dashboards/${id}`, method: 'delete' })
}
export function publicViewScreen(code) {
  return request({ url: `/api/dashboards/public/${code}`, method: 'get' })
}

// 真实大屏数据接口(后端聚合) - 严禁使用模拟数据
export function getDashboardData() {
  return request({ url: '/api/dashboards/data', method: 'get' })
}
export function getRealtimeActivities() {
  return request({ url: '/api/dashboards/activities', method: 'get' })
}
export function getKPIs() {
  return request({ url: '/api/dashboards/data', method: 'get' })
}
export function getTrends(params) {
  return request({ url: '/api/dashboards/data', method: 'get', params })
}
export function getChannels() {
  return request({ url: '/api/dashboards/data', method: 'get' })
}
export function getRegions() {
  return request({ url: '/api/dashboards/data', method: 'get' })
}
