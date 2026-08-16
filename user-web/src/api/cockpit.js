import { http } from '@/utils/request'

// 运维总览（OPT-UX-01）
export function getSystemStatus() {
  return http.get('/monitor/system-status')
}

export function getOpsStats() {
  return http.get('/monitor/ops-stats')
}

export function getModuleStatus() {
  return http.get('/monitor/module-status')
}

export function getRecentOperations(limit = 20) {
  return http.get('/monitor/recent-ops', { params: { limit } })
}

// AI 销冠驾驶舱（OPT-UX-02）
export function getSalesCockpit() {
  return http.get('/ai/sales-cockpit')
}
