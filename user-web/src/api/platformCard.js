import { http } from '@/utils/request'

const PLATFORMS = ['douyin', 'kuaishou', 'xiaohongshu', 'xianyu', 'tiktok'];

const BASE_PATHS = {
  douyin: '/api/douyin-card',
  kuaishou: '/api/kuaishou-card',
  xiaohongshu: '/api/xiaohongshu-card',
  xianyu: '/api/xianyu-card',
  tiktok: '/api/tiktok-cards'
};
const basePath = (platform) => BASE_PATHS[platform] || `/api/${platform}-card`

export const listCards = (platform, params) => http.get(`${basePath(platform)}/list`, params)
export const getCard = (platform, id) => http.get(`${basePath(platform)}/${id}`)
export const createCard = (platform, data) => http.post(basePath(platform), { ...data, platform })
export const updateCard = (platform, id, data) => http.put(`${basePath(platform)}/${id}`, data)
export const deleteCard = (platform, id) => http.delete(`${basePath(platform)}/${id}`)
export const getCardStats = (platform, id) => http.get(`/api/${platform}-card/stats/${id}`)
export const publishToPlatform = (platform, data) =>
  http.post('/api/cards/cross-publish', { platform, data })

export const crossPublish = (data, platforms) =>
  http.post('/api/cards/cross-publish', { data, platforms });
