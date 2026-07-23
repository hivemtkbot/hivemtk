import request from '@/utils/request'

// 团队用户 - 匹配后端 /api/team/* 路径
export function getTeamUserList(params) {
  return request({ url: '/api/team/users', method: 'get', params })
}
export function getTeamUser(id) {
  return request({ url: `/api/team/users/${id}`, method: 'get' })
}
export function createTeamUser(data) {
  return request({ url: '/api/team/users', method: 'post', data })
}
export function updateTeamUser(id, data) {
  return request({ url: `/api/team/users/${id}`, method: 'put', data })
}
export function deleteTeamUser(id) {
  return request({ url: `/api/team/users/${id}`, method: 'delete' })
}
export function resetTeamUserPassword(id) {
  return request({ url: `/api/team/users/${id}/reset-password`, method: 'post' })
}
export function getCurrentTeamUser() {
  return request({ url: '/api/team/user/current', method: 'get' })
}
export function changeTeamUserPassword(data) {
  return request({ url: '/api/team/user/change-password', method: 'post', data })
}

// 团队角色
export function getTeamRoleList() {
  return request({ url: '/api/team/roles', method: 'get' })
}
export function createTeamRole(data) {
  return request({ url: '/api/team/roles', method: 'post', data })
}
export function updateTeamRole(id, data) {
  return request({ url: `/api/team/roles/${id}`, method: 'put', data })
}
export function deleteTeamRole(id) {
  return request({ url: `/api/team/roles/${id}`, method: 'delete' })
}
export function getPermissions() {
  return request({ url: '/api/team/permissions', method: 'get' })
}

// 兼容旧接口
export function getTeamMembers(params) {
  return getTeamUserList(params)
}
export function createTeamMember(data) {
  return createTeamUser(data)
}
export function updateTeamMember(id, data) {
  return updateTeamUser(id, data)
}
export function deleteTeamMember(id) {
  return deleteTeamUser(id)
}
export function resetTeamPassword(id) {
  return resetTeamUserPassword(id)
}
export function getTeamStats() {
  return request({ url: '/api/team/logs/statistics', method: 'get' })
}
export function assignRole(id, data) {
  return updateTeamUser(id, { ...data, roleId: data.roleId })
}
