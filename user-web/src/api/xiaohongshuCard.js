import { http } from '@/utils/request'

export function getXiaohongshuCardList(params) {
  return http.get('/api/xiaohongshu/list', params)
}

export function getXiaohongshuCard(id) {
  return http.get(`/api/xiaohongshu/${id}`)
}

export function createXiaohongshuCard(data) {
  return http.post('/api/xiaohongshu/create', data)
}

export function updateXiaohongshuCard(data) {
  return http.put('/api/xiaohongshu/update', data)
}

export function deleteXiaohongshuCard(id) {
  return http.delete(`/api/xiaohongshu/delete/${id}`)
}

export function viewXiaohongshuCard(id) {
  return http.get(`/api/xiaohongshu/view/${id}`)
}

export function getXiaohongshuCardStats(id, params) {
  return http.get(`/api/xiaohongshu/stats/card/${id}`, params)
}

export function getXiaohongshuCardOverallStats(params) {
  return http.get('/api/xiaohongshu/stats/overall', params)
}

export function generateShortLink(id) {
  return http.post(`/api/xiaohongshu/${id}/generate-short-link`)
}