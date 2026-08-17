/**
 * TikTok卡片API
 */

import { http } from '@/utils/request';

// 获取TikTok卡片列表
export function getTikTokCardList(params) {
  return http.get('/api/tiktok-card/list', params)
}

// 获取TikTok卡片详情
export function getTikTokCard(id) {
  return http.get(`/api/tiktok-card/${id}`)
}

// 创建TikTok卡片
export function createTikTokCard(data) {
  return http.post('/api/tiktok-card', data)
}

// 更新TikTok卡片
export function updateTikTokCard(data) {
  return http.put(`/api/tiktok-card/${data.id}`, data)
}

// 删除TikTok卡片
export function deleteTikTokCard(id) {
  return http.delete(`/api/tiktok-card/${id}`)
}

// 生成短链接
export function generateShortLink(cardId) {
  return http.post('/api/tiktok-card/generate-short-link', { card_id: cardId })
}

// 获取总体统计信息
export function getTikTokCardOverallStats(params) {
  return http.get('/api/tiktok-card/stats/overall', params)
}

// 获取单个卡片统计信息
export function getTikTokCardStats(cardId, params) {
  return http.get(`/api/tiktok-card/${cardId}/stats`, params)
}