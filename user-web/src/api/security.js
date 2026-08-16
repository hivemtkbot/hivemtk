import { http } from '@/utils/request'

/**
 * 安全审计（USR-SY-04）
 * 风险评估：密码强度/异常登录/越权访问
 */

export const getSecurityOverview = () => http.get('/api/security/overview')
export const listAbnormalLogins = (params) => http.get('/api/security/abnormal-logins', params)
export const listUnauthorizedAccess = (params) => http.get('/api/security/unauthorized-access', params)
export const listPasswordWeak = () => http.get('/api/security/password-weak')
export const generateSecurityReport = (params) => http.post('/api/security/report/generate', params)
export const getSecurityReport = (id) => http.get(`/api/security/report/${id}`)
export const triggerScan = (scanType) => http.post('/api/security/scan', { type: scanType })
