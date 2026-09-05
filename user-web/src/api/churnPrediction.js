import { http } from '@/utils/request';

export function getChurnPrediction(params) {
  return http.get('/api/churn/prediction', params)
}
export function getChurnPredictions(params) {
  return http.get('/api/churn-prediction', params)
}
export function getHighRiskUsers(params) {
  return http.get('/api/churn-prediction/users', params)
}
export function getChurnWarnings(params) {
  return http.get('/api/churn-prediction/warnings', params)
}
export function getUnhandledWarnings(params) {
  return http.get('/api/churn/unhandled-warnings', params)
}
export function markWarningHandled(id, data) {
  return http.post(`/api/churn/warnings/${id}/handle`, data)
}
export function getChurnModelConfig() {
  return http.get('/api/churn-prediction/model-config')
}
export function saveChurnModelConfig(data) {
  return http.post('/api/churn-prediction/model-config', data)
}
export function getChurnStatistics(params) {
  return http.get('/api/churn-prediction/statistics', params)
}
export function getRiskDistribution() {
  return http.get('/api/churn/risk-distribution')
}

export function runChurnPrediction() {
  return calculateRFM({ type: 'churn' })
}
export function interveneUser(data) {
  return http.post('/api/churn/warnings/intervene', data)
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
  return http.post('/api/user-segment/rfm/calculate', data)
}
