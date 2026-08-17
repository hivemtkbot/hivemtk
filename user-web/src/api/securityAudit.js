import { http } from '@/utils/request';

// 启动安全审计
export function runSecurityAudit(data) {
  return http.post('/api/security/audit', data)
}

// 审计历史列表
export function getSecurityAuditList(params) {
  return http.get('/api/security/audit/list', params)
}

// 审计详情
export function getSecurityAuditDetail(id) {
  return http.get(`/api/security/audit/${id}`)
}
