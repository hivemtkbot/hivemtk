import { http } from '@/utils/request'

/**
 * 短链 + 活码统一抽象（USR-RC-05）
 * type: 'short' | 'qrcode'
 */

// 统一 links API
export const listLinks = (params) => http.get('/api/links', params)
export const createShortLink = (data) => http.post('/api/shortlinks', data)
export const createQRLink = (data) => http.post('/api/qrcodes', data)
export const getLinkStats = (id) => http.get(`/api/links/${id}/stats`)
export const updateLink = (id, data) => http.put(`/api/links/${id}`, data)
export const deleteLink = (id) => http.delete(`/api/links/${id}`)

// 域名池健康度
export const getDomainPoolHealth = () => http.get('/api/domain-pool/health')
export const suspendDomain = (id) => http.post(`/api/domain-pool/${id}/suspend`, {})

// 短链/活码切换
export const convertLinkType = (id, targetType) =>
  http.post(`/api/links/${id}/convert`, { target_type: targetType })
