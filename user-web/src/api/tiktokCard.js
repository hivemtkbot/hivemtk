/**
 * TikTok卡片API
 */

import { http } from '@/utils/request';

// 获取TikTok卡片列表
export function getTikTokCardList(params) {
  return request({
    url: '/api/tiktok-card/list',
    method: 'get',
    params
  })
}

// 获取TikTok卡片详情
export function getTikTokCard(id) {
  return http.get(`/api/tiktok-card/${id}`)
}

// 创建TikTok卡片
export function createTikTokCard(data) {
  return request({
    url: '/api/tiktok-card',
    method: 'post',
    data
  })
}

// 更新TikTok卡片
export function updateTikTokCard(data) {
  return request({
    url: `/api/tiktok-card/${data.id}`,
    method: 'put',
    data
  })
}

// 删除TikTok卡片
export function deleteTikTokCard(id) {
  return http.delete(`/api/tiktok-card/${id}`)
}

// 生成短链接
export function generateShortLink(cardId) {
  return request({
    url: '/api/tiktok-card/generate-short-link',
    method: 'post',
    data: { card_id: cardId }
  })
}

// 获取总体统计信息
export function getTikTokCardOverallStats(params) {
  return request({
    url: '/api/tiktok-card/stats/overall',
    method: 'get',
    params
  })
}

// 获取单个卡片统计信息
export function getTikTokCardStats(cardId, params) {
  return request({
    url: `/api/tiktok-card/${cardId}/stats`,
    method: 'get',
    params
  })
}