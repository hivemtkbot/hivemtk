// systemUser.js 系统用户（人员管理）API
//
// 阶段 4：人员管理全栈
// 路由前缀：/api/system/users
// 后端：router/system_user_routes.go（受 RequireAdminMiddleware 保护）
//
// 命名规范：与 users.js / operationLog.js 等保持一致，使用默认导出 + 命名导出
import { http } from '@/utils/request'

// 列表查询参数
//   keyword: 模糊搜索 username/email/real_name
//   role:    角色过滤（admin / customer_service / staff）
//   page:    页码（1-based）
//   size:    每页条数
export function listSystemUsers(params) {
  return http.get('/api/system/users', { params })
}

// 单个详情
export function getSystemUser(id) {
  return http.get(`/api/system/users/${id}`)
}

// 创建账号
export function createSystemUser(data) {
  return http.post('/api/system/users', data)
}

// 更新账号（字段均为可选，仅传需要改的）
//   { username?, email?, name?, role? }
export function updateSystemUser(id, data) {
  return http.put(`/api/system/users/${id}`, data)
}

// 删除账号
export function deleteSystemUser(id) {
  return http.delete(`/api/system/users/${id}`)
}

// 命名导出聚合对象（兼容部分页面直接引用）
export const systemUserApi = {
  list: listSystemUsers,
  get: getSystemUser,
  create: createSystemUser,
  update: updateSystemUser,
  remove: deleteSystemUser
}

export default systemUserApi
