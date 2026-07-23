import request from '@/utils/request'

// 流失预警 - 匹配后端真实路由 /api/churn-prediction/*
export function getChurnPrediction(params) {
  return request({ url: '/api/churn/prediction', method: 'get', params })
}
export function getChurnPredictions(params) {
  return request({ url: '/api/churn-prediction', method: 'get', params })
}
export function getHighRiskUsers(params) {
  return request({ url: '/api/churn-prediction/users', method: 'get', params })
}
export function getChurnWarnings(params) {
  return request({ url: '/api/churn-prediction/warnings', method: 'get', params })
}
export function getUnhandledWarnings(params) {
  return request({ url: '/api/churn-prediction/unhandled-warnings', method: 'get', params })
}
export function markWarningHandled(id, data) {
  return request({ url: `/api/churn/warnings/${id}/handle`, method: 'post', data })
}
export function getChurnModelConfig() {
  return request({ url: '/api/churn-prediction/model-config', method: 'get' })
}
export function saveChurnModelConfig(data) {
  return request({ url: '/api/churn-prediction/model-config', method: 'post', data })
}
export function getChurnStatistics(params) {
  return request({ url: '/api/churn-prediction/statistics', method: 'get', params })
}
export function getRiskDistribution() {
  return request({ url: '/api/churn-prediction/risk-distribution', method: 'get' })
}

// 兼容旧接口
export function runChurnPrediction() {
  return calculateRFM({ type: 'churn' })
}
export function interveneUser(data) {
  return request({ url: '/api/churn/warnings/intervene', method: 'post', data })
}
export function getChurnStats(params) {
  return getChurnStatistics(params)
}
export function getChurnConfig() {
  return getChurnModelConfig()
}
export function updateChurnConfig(data) {
  return saveChurnModelConfig(data)
}
function calculateRFM(data) {
  return request({ url: '/api/user-segment/rfm/calculate', method: 'post', data })
}
