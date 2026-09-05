import { http } from '@/utils/request'

export function getDouyinCardList(params) {
  return http.get('/api/douyin/list', params)
}

export function getDouyinCard(id) {
  return http.get(`/api/douyin/${id}`)
}

export function createDouyinCard(data) {
  return http.post('/api/douyin/create', data)
}

export function updateDouyinCard(data) {
  return http.put('/api/douyin/update', data)
}

export function deleteDouyinCard(id) {
  return http.delete(`/api/douyin/delete/${id}`)
}

export function viewDouyinCard(id) {
  return http.get(`/api/douyin/view/${id}`)
}

export function getDouyinCardStats(id, params) {
  return http.get(`/api/douyin/stats/card/${id}`, params)
}

export function getDouyinCardOverallStats(params) {
  return http.get('/api/douyin/stats/overall', params)
}

export function generateShortLink(id) {
  return http.post(`/api/douyin/${id}/generate-short-link`)
}