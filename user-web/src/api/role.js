import { http } from '@/utils/request';

export function listRoles() {
  return http.get('/api/system/roles')
}

export function listSystemRoles(params = {}) {
  return http.get('/api/system/roles', {
    params: { ...params, role_type: 'system' }
  })
}

export function listCustomRoles(params = {}) {
  return http.get('/api/system/roles', {
    params: { ...params, role_type: 'custom' }
  })
}

export function getRole(code) {
  return http.get(`/api/system/roles/${code}`)
}

export function createRole(data) {
  return http.post('/api/system/roles', data)
}

export function updateRole(code, data) {
  return http.put(`/api/system/roles/${code}`, data)
}

export function deleteRole(code) {
  return http.delete(`/api/system/roles/${code}`)
}

export function listRoleMembers(code, params) {
  return http.get(`/api/system/roles/${code}/members`, { params })
}

export function getMenuTree() {
  return http.get('/api/system/menus')
}

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
};

export default roleApi
