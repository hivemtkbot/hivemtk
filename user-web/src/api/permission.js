import { http } from '@/utils/request';

export function setUserEnabled(id, enabled) {
  return http.put(`/api/system/permissions/${id}/enabled`, { enabled })
}

export function resetUserPassword(id, password) {
  return http.put(`/api/system/permissions/${id}/password`, { password })
}

export function listAuditLogs(params) {
  return http.get('/api/system/permissions/audit-logs', { params })
}

export const permissionApi = {
  setEnabled: setUserEnabled,
  resetPassword: resetUserPassword,
  listAuditLogs
};

export default permissionApi
