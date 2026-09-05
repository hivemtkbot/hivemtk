import { http } from '@/utils/request'

export function getSystemStatus() {
  return http.get('/api/system/stats')
}

export function getOpsStats() {
  return http.get('/api/monitor/health')
}

export function getModuleStatus() {
  return http.get('/api/monitor/node-health')
}

export function getRecentOperations(limit = 20) {
  return http.get('/api/monitor/anomalies', { limit })
}

export function getSalesCockpit() {
  return http.get('/api/monitor/health')
}
