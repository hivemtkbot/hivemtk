import { http } from '@/utils/request';

// 启动安全审计
export function runSecurityAudit(data) {
  return request({ url: '/api/security/audit', method: 'post', data })
}

// 审计历史列表
export function getSecurityAuditList(params) {
  return request({ url: '/api/security/audit/list', method: 'get', params })
}

// 审计详情
export function getSecurityAuditDetail(id) {
  return http.get(`/api/security/audit/${id}`)
}
