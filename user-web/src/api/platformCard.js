import { http } from '@/utils/request'

/**
 * 5 平台卡片统一 API（USR-RC-01）
 * 抽象层：所有平台用同一个 API，按 platform 参数路由
 */

const PLATFORMS = ['douyin', 'kuaishou', 'xiaohongshu', 'xianyu', 'tiktok']

// R39 修复：后端实际路由为 kebab-case（/api/douyin-card 等），
// tiktok 为复数 /api/tiktok-cards —— 按平台解析路径前缀（修复 404）
const BASE_PATHS = {
  douyin: '/api/douyin-card',
  kuaishou: '/api/kuaishou-card',
  xiaohongshu: '/api/xiaohongshu-card',
  xianyu: '/api/xianyu-card',
  tiktok: '/api/tiktok-cards'
}
const basePath = (platform) => BASE_PATHS[platform] || `/api/${platform}-card`

export const listCards = (platform, params) => http.get(`${basePath(platform)}/list`, params)
export const getCard = (platform, id) => http.get(`${basePath(platform)}/${id}`)
export const createCard = (platform, data) => http.post(basePath(platform), { ...data, platform })
export const updateCard = (platform, id, data) => http.put(`${basePath(platform)}/${id}`, data)
export const deleteCard = (platform, id) => http.delete(`${basePath(platform)}/${id}`)
export const getCardStats = (platform, id) => http.get(`/api/${platform}-card/stats/${id}`)
export const publishToPlatform = (platform, data) =>
  http.post('/api/cards/cross-publish', { platform, data })

// 跨平台发布（一次性发到 5 个平台）
export const crossPublish = (data, platforms) =>
  http.post('/api/cards/cross-publish', { data, platforms })
