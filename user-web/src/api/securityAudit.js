import { http } from '@/utils/request';

export function runSecurityAudit(data) {
  return http.post('/api/security/audit', data)
}

export function getSecurityAuditList(params) {
  return http.get('/api/security/audit/list', params)
}

export function getSecurityAuditDetail(id) {
  return http.get(`/api/security/audit/${id}`)
}
