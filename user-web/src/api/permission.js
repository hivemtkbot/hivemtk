// permission.js 授权管理 API
//
// 阶段 6：授权管理全栈
// 路由前缀：/api/system/permissions
// 后端：router/permission_routes.go（受 RequireAdminMiddleware 保护）
import { http } from '@/utils/request'

// 启用/禁用账号
//   enabled: true 启用 / false 禁用
export function setUserEnabled(id, enabled) {
  return http.put(`/api/system/permissions/${id}/enabled`, { enabled })
}

// 重置密码（admin 操作）
//   password: 新明文密码（后端做强度校验）
export function resetUserPassword(id, password) {
  return http.put(`/api/system/permissions/${id}/password`, { password })
}

// 操作审计日志
//   params: { user_id?, action?, page?, page_size? }
export function listAuditLogs(params) {
  return http.get('/api/system/permissions/audit-logs', { params })
}

// 命名导出聚合对象
export const permissionApi = {
  setEnabled: setUserEnabled,
  resetPassword: resetUserPassword,
  listAuditLogs
}

export default permissionApi
