import { http } from '@/utils/request'

export function getXianyuCardList(params) {
  return http.get('/api/xianyu/list', params)
}

export function getXianyuCard(id) {
  return http.get(`/api/xianyu/${id}`)
}

export function createXianyuCard(data) {
  return http.post('/api/xianyu/create', data)
}

export function updateXianyuCard(data) {
  return http.put('/api/xianyu/update', data)
}

export function deleteXianyuCard(id) {
  return http.delete(`/api/xianyu/delete/${id}`)
}

export function viewXianyuCard(id) {
  return http.get(`/api/xianyu/view/${id}`)
}

export function getXianyuCardStats(id, params) {
  return http.get(`/api/xianyu/stats/card/${id}`, params)
}

export function getXianyuCardOverallStats(params) {
  return http.get('/api/xianyu/stats/overall', params)
}

export function generateXianyuShortLink(id) {
  return http.post(`/api/xianyu/${id}/generate-short-link`)
}