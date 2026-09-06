import { http } from '@/utils/request';

export function getExperimentList(params) {
  return http.get('/api/ab-experiments', params)
}
export function getExperiment(id) {
  return http.get(`/api/ab-experiments/${id}`)
}
export function createExperiment(data) {
  return http.post('/api/ab-experiments', data)
}
export function updateExperiment(id, data) {
  return http.put(`/api/ab-experiments/${id}`, data)
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
