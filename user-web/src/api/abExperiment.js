import request from '@/utils/request'

// A/B 测试 - 匹配后端 /api/ab-experiments/* 路径
export function getExperimentList(params) {
  return request({ url: '/api/ab-experiments', method: 'get', params })
}
export function getExperiment(id) {
  return request({ url: `/api/ab-experiments/${id}`, method: 'get' })
}
export function createExperiment(data) {
  return request({ url: '/api/ab-experiments', method: 'post', data })
}
export function updateExperiment(id, data) {
  return request({ url: `/api/ab-experiments/${id}`, method: 'put', data })
}
export function deleteExperiment(id) {
  return request({ url: `/api/ab-experiments/${id}`, method: 'delete' })
}
export function startExperiment(id) {
  return request({ url: `/api/ab-experiments/${id}/start`, method: 'post' })
}
export function pauseExperiment(id) {
  return request({ url: `/api/ab-experiments/${id}/pause`, method: 'post' })
}
export function stopExperiment(id) {
  return request({ url: `/api/ab-experiments/${id}/stop`, method: 'post' })
}
export function getExperimentResults(id) {
  return request({ url: `/api/ab-experiments/${id}/results`, method: 'get' })
}
export function getConversionEvents(id) {
  return request({ url: `/api/ab-experiments/${id}/conversion-events`, method: 'get' })
}

// 兼容旧接口
export function getExperiments(params) {
  return getExperimentList(params)
}
export function resumeExperiment(id) {
  return startExperiment(id)
}
export function getExperimentStats() {
  return request({ url: '/api/ab-experiments', method: 'get' })
}
export function getExperimentDetail(id) {
  return getExperiment(id)
}
