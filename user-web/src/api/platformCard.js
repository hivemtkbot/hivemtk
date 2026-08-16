import { http } from '@/utils/request'

/**
 * 5 平台卡片统一 API（USR-RC-01）
 * 抽象层：所有平台用同一个 API，按 platform 参数路由
 */

const PLATFORMS = ['douyin', 'kuaishou', 'xiaohongshu', 'xianyu', 'tiktok']

export const listCards = (platform, params) => http.get(`/api/${platform}Card`, params)
export const getCard = (platform, id) => http.get(`/api/${platform}Card/${id}`)
export const createCard = (platform, data) => http.post(`/api/${platform}Card`, { ...data, platform })
export const updateCard = (platform, id, data) => http.put(`/api/${platform}Card/${id}`, data)
export const deleteCard = (platform, id) => http.delete(`/api/${platform}Card/${id}`)
export const getCardStats = (platform, id) => http.get(`/api/${platform}Card/${id}/stats`)
export const publishToPlatform = (platform, data) =>
  http.post('/api/cards/cross-publish', { platform, data })

// 跨平台发布（一次性发到 5 个平台）
export const crossPublish = (data, platforms) =>
  http.post('/api/cards/cross-publish', { data, platforms })
