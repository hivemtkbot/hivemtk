import { http } from '@/utils/request';

// 用户分群(RFM) - 匹配后端 /api/user-segment/* 路径
export function getRFMList(params) {
  return request({ url: '/api/user-segment/rfm/list', method: 'get', params })
}
export function getRFMRule() {
  return http.get('/api/user-segment/rfm/rule')
}
export function saveRFMRule(data) {
  return request({ url: '/api/user-segment/rfm/rule', method: 'post', data })
}
export function updateRFMRule(id, data) {
  return request({ url: `/api/user-segment/rfm/rule/${id}`, method: 'put', data })
}
export function getUserRFM(userId) {
  return http.get(`/api/user-segment/rfm/user?user_id=${userId}`)
}
export function getRFMStats() {
  return http.get('/api/user-segment/rfm/stats')
}
export function calculateRFM(data) {
  return request({ url: '/api/user-segment/rfm/calculate', method: 'post', data })
}
export function getLayerDescription() {
  return http.get('/api/user-segment/layers')
}

// 兼容旧接口
export function getUserSegments(params) {
  return request({ url: '/api/user-segments', method: 'get', params })
}
export function getSegmentStats() {
  return getRFMStats()
}
export function createUserSegment(data) {
  return saveRFMRule(data)
}
export function updateUserSegment(id, data) {
  return updateRFMRule(id, data)
}
export function deleteUserSegment(id) {
  return http.delete(`/api/user-segment/rfm/rule/${id}`)
}
export function getSegmentUsers(id, params) {
  return request({ url: `/api/user-segment/rfm/list?segment_id=${id}`, method: 'get', params })
}
