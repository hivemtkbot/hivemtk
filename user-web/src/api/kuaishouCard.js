import { http } from '@/utils/request'

export function getKuaishouCardList(params) {
  return http.get('/api/kuaishou/list', params)
}

export function getKuaishouCard(id) {
  return http.get(`/api/kuaishou/${id}`)
}

export function createKuaishouCard(data) {
  return http.post('/api/kuaishou/create', data)
}

export function updateKuaishouCard(data) {
  return http.put('/api/kuaishou/update', data)
}

export function deleteKuaishouCard(id) {
  return http.delete(`/api/kuaishou/delete/${id}`)
}

export function viewKuaishouCard(id) {
  return http.get(`/api/kuaishou/view/${id}`)
}

export function likeKuaishouCard(id) {
  return http.post(`/api/kuaishou/like/${id}`)
}

export function shareKuaishouCard(id) {
  return http.post(`/api/kuaishou/share/${id}`)
}

export function getKuaishouCardStats(id, params) {
  return http.get(`/api/kuaishou/stats/card/${id}`, params)
}

export function getKuaishouCardOverallStats(params) {
  return http.get('/api/kuaishou/stats/overall', params)
}

export function generateShortLink(id) {
  return http.post(`/api/kuaishou/${id}/generate-short-link`)
}