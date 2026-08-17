import { http } from '@/utils/request'

/**
 * 域名池健康度预警（USR-SM-04）
 * 借鉴：domain-monitoring 实践
 */

export const getDomainHealth = () => http.get('/api/domain-pool/health')
export const checkBlacklist = (domain) => http.get(`/api/domain-pool/${domain}/blacklist`)
export const suspendDomain = (id) => http.post(`/api/domain-pool/${id}/suspend`, {})
export const rotateToBackup = (id) => http.post(`/api/domain-pool/${id}/rotate`, {})
export const listAlerts = (params) => http.get('/api/domain-pool/alerts', params)
export const resolveAlert = (id) => http.post(`/api/domain-pool/alerts/${id}/resolve`, {})

export const domainPoolApi = {
  getList: (params) => http.get('/api/domain-pool', params),
  create: (data) => http.post('/api/domain-pool', data),
  update: (data) => http.put(`/api/domain-pool/${data.id}`, data),
  delete: (id) => http.delete(`/api/domain-pool/${id}`),
  checkDomain: (id) => http.post(`/api/domain-pool/${id}/check`, {}),
  checkAllDomains: () => http.post('/api/domain-pool/check-all', {}),
}
