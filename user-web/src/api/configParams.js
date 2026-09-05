import { http } from '@/utils/request'

export function list(group) {
  return http.get('/api/manage/config-params', { params: { group } })
}

export function auditLogs(limit = 100) {
  return http.get('/api/manage/config-params/audit-logs', { params: { limit } })
}

export function update(group, key, value) {
  return http.put(`/api/manage/config-params/${group}/${key}`, { value })
}

export function reset(group, key) {
  return http.post(`/api/manage/config-params/${group}/${key}/reset`)
}

export function bulkReset(group) {
  return http.post(`/api/manage/config-params/${group}/reset`)
}
