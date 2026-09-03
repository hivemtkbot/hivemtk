import { http } from '@/utils/request'

/**
 * 动态阈值参数管理（ConfigParams）
 *
 * 后端路由：/api/manage/config-params*  （user-server）
 * 种子定义：internal/service/config_param_seeds.go（109 条，18 分组）
 */

// 获取全部参数（可按 group 过滤）
export function list(group) {
  return http.get('/api/manage/config-params', { params: { group } })
}

// 获取变更审计日志
export function auditLogs(limit = 100) {
  return http.get('/api/manage/config-params/audit-logs', { params: { limit } })
}

// 更新单个参数值
export function update(group, key, value) {
  return http.put(`/api/manage/config-params/${group}/${key}`, { value })
}

// 重置单个参数到默认值
export function reset(group, key) {
  return http.post(`/api/manage/config-params/${group}/${key}/reset`)
}

// 整组批量重置到默认值
export function bulkReset(group) {
  return http.post(`/api/manage/config-params/${group}/reset`)
}
