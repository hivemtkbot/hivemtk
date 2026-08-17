import { http } from '@/utils/request';

// 获取活码列表
export function getLiveCodes(params) {
  return http.get('/api/live-codes/list', params)
}

// 获取活码详情
export function getLiveCode(id) {
  return http.get(`/api/live-codes/${id}`)
}

// 创建活码
export function createLiveCode(data) {
  return http.post('/api/live-codes/create', data)
}

// 更新活码
export function updateLiveCode(id, data) {
  return http.put(`/api/live-codes/${id}/update`, data)
}

// 删除活码
export function deleteLiveCode(id) {
  return http.delete(`/api/live-codes/${id}/delete`)
}

// 获取活码统计
export function getLiveCodeStats(id) {
  return http.get(`/api/live-codes/${id}/stats`)
}

// 获取活码二维码列表
export function getLiveCodeQRs(liveCodeId) {
  return http.get(`/api/live-codes/${liveCodeId}/qrcodes`)
}

// 生成活码二维码
export function generateLiveCodeQR(liveCodeId, data) {
  return http.post(`/api/live-codes/${liveCodeId}/qrcodes/create`, data)
}

// 获取活码二维码统计
export function getLiveCodeQRStats(qrId) {
  return http.get(`/api/live-codes/qrcodes/${qrId}/stats`)
}

// 分享活码
export function shareLiveCode(id, data) {
  return http.post(`/api/live-codes/${id}/share`, data)
}

// 删除活码二维码
export function deleteLiveCodeQR(id) {
  return http.delete(`/api/live-codes/qrcodes/${id}/delete`)
}

// 更新活码二维码
export function updateLiveCodeQR(id, data) {
  return http.put(`/api/live-codes/qrcodes/${id}/update`, data)
}