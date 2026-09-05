import { http } from '@/utils/request';

export function getTikTokCardList(params) {
  return http.get('/api/tiktok-card/list', params)
}

export function getTikTokCard(id) {
  return http.get(`/api/tiktok-card/${id}`)
}

export function createTikTokCard(data) {
  return http.post('/api/tiktok-card', data)
}

export function updateTikTokCard(data) {
  return http.put(`/api/tiktok-card/${data.id}`, data)
}

export function deleteTikTokCard(id) {
  return http.delete(`/api/tiktok-card/${id}`)
}

export function generateShortLink(cardId) {
  return http.post('/api/tiktok-card/generate-short-link', { card_id: cardId })
}

export function getTikTokCardOverallStats(params) {
  return http.get('/api/tiktok-card/stats/overall', params)
}

export function getTikTokCardStats(cardId, params) {
  return http.get(`/api/tiktok-card/${cardId}/stats`, params)
}