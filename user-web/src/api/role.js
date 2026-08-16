// role.js 角色管理 API（OPT-UX-07 增强）
//
// 阶段 5：角色管理全栈
// 路由前缀：/api/system/roles
// 后端：router/role_routes.go（受 RequireAdminMiddleware 保护）
//
// 新增（OPT-UX-07 自定义角色 v3.1）：
//   - listSystemRoles / listCustomRoles
//   - createRole / updateRole / deleteRole
//   - 数据范围 / 菜单权限 / 按钮权限 CRUD
import { http } from '@/utils/request'

// 列出 3 档系统角色 + 成员数
export function listRoles() {
  return http.get('/api/system/roles')
}

// 列出系统预置角色（admin / customer_service / staff）
// OPT-UX-07：分页 1，大小 100
export function listSystemRoles(params = {}) {
  return http.get('/api/system/roles', {
    params: { ...params, role_type: 'system' }
  })
}

// 列出商户自定义角色
// OPT-UX-07：分页
export function listCustomRoles(params = {}) {
  return http.get('/api/system/roles', {
    params: { ...params, role_type: 'custom' }
  })
}

// 单个角色详情（带成员数）
export function getRole(code) {
  return http.get(`/api/system/roles/${code}`)
}

// 创建自定义角色
// OPT-UX-07：role_code / name / color / icon / menu_perms / button_perms / scope_type
export function createRole(data) {
  return http.post('/api/system/roles', data)
}

// 更新自定义角色
export function updateRole(code, data) {
  return http.put(`/api/system/roles/${code}`, data)
}

// 删除自定义角色
export function deleteRole(code) {
  return http.delete(`/api/system/roles/${code}`)
}

// 角色下成员列表（分页）
//   page: 1-based
//   size: 每页条数
export function listRoleMembers(code, params) {
  return http.get(`/api/system/roles/${code}/members`, { params })
}

// OPT-UX-07：菜单权限树（用于角色编辑）
export function getMenuTree() {
  return http.get('/api/system/menus')
}

// 命名导出聚合对象
export const roleApi = {
  list: listRoles,
  listSystem: listSystemRoles,
  listCustom: listCustomRoles,
  get: getRole,
  create: createRole,
  update: updateRole,
  delete: deleteRole,
  listMembers: listRoleMembers,
  getMenuTree
}

export default roleApi
