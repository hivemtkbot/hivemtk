import { http } from '@/utils/request';

// A/B 测试 - 匹配后端 /api/ab-experiments/* 路径
export function getExperimentList(params) {
  return request({ url: '/api/ab-experiments', method: 'get', params })
}
export function getExperiment(id) {
  return http.get(`/api/ab-experiments/${id}`)
}
export function createExperiment(data) {
  return request({ url: '/api/ab-experiments', method: 'post', data })
}
export function updateExperiment(id, data) {
  return request({ url: `/api/ab-experiments/${id}`, method: 'put', data })
}
export function deleteExperiment(id) {
  return http.delete(`/api/ab-experiments/${id}`)
}
export function startExperiment(id) {
  return http.post(`/api/ab-experiments/${id}/start`)
}
export function pauseExperiment(id) {
  return http.post(`/api/ab-experiments/${id}/pause`)
}
export function stopExperiment(id) {
  return http.post(`/api/ab-experiments/${id}/stop`)
}
export function getExperimentResults(id) {
  return http.get(`/api/ab-experiments/${id}/results`)
}
export function getConversionEvents(id) {
  return http.get(`/api/ab-experiments/${id}/conversion-events`)
}

// 兼容旧接口
export function getExperiments(params) {
  return getExperimentList(params)
}
export function resumeExperiment(id) {
  return startExperiment(id)
}
export function getExperimentStats() {
  return http.get('/api/ab-experiments')
}
export function getExperimentDetail(id) {
  return getExperiment(id)
}
