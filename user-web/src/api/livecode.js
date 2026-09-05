import { http } from '@/utils/request';

export function getLiveCodes(params) {
  return http.get('/api/live-codes/list', params)
}

export function getLiveCode(id) {
  return http.get(`/api/live-codes/${id}`)
}

export function createLiveCode(data) {
  return http.post('/api/live-codes/create', data)
}

export function updateLiveCode(id, data) {
  return http.put(`/api/live-codes/${id}/update`, data)
}

export function deleteLiveCode(id) {
  return http.delete(`/api/live-codes/${id}/delete`)
}

export function getLiveCodeStats(id) {
  return http.get(`/api/live-codes/${id}/stats`)
}

export function getLiveCodeQRs(liveCodeId) {
  return http.get(`/api/live-codes/${liveCodeId}/qrcodes`)
}

export function generateLiveCodeQR(liveCodeId, data) {
  return http.post(`/api/live-codes/${liveCodeId}/qrcodes/create`, data)
}

export function getLiveCodeQRStats(qrId) {
  return http.get(`/api/live-codes/qrcodes/${qrId}/stats`)
}

export function shareLiveCode(id, data) {
  return http.post(`/api/live-codes/${id}/share`, data)
}

export function deleteLiveCodeQR(id) {
  return http.delete(`/api/live-codes/qrcodes/${id}/delete`)
}

export function updateLiveCodeQR(id, data) {
  return http.put(`/api/live-codes/qrcodes/${id}/update`, data)
}