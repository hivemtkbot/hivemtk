import { http } from '@/utils/request';

export function getConfidenceSignals(params) {
  return http.get('/api/admin/tuning/confidence/signals', params)
}
export function getConfidenceSignal(id) {
  return http.get(`/api/admin/tuning/confidence/signals/${id}`)
}
export function getConfidenceSignalStats(params) {
  return http.get('/api/admin/tuning/confidence/signals/stats', params)
}
export function getConfidenceCalibrations(params) {
  return http.get('/api/admin/tuning/confidence/calibrations', params)
}
export function getThresholdPolicies(params) {
  return http.get('/api/admin/tuning/confidence/policies', params)
}
export function upsertThresholdPolicy(data) {
  return http.put('/api/admin/tuning/confidence/policies', data)
}

export function getHumanizeScores(params) {
  return http.get('/api/admin/tuning/humanize/scores', params)
}
export function getHumanizeScoreStats(params) {
  return http.get('/api/admin/tuning/humanize/scores/stats', params)
}
export function getChampionBaselines(params) {
  return http.get('/api/admin/tuning/humanize/baselines', params)
}
export function getLowQualitySamples(params) {
  return http.get('/api/admin/tuning/humanize/low-quality', params)
}

export function getFeedbackEvents(params) {
  return http.get('/api/admin/tuning/feedback/events', params)
}
export function getFeedbackEventStats(params) {
  return http.get('/api/admin/tuning/feedback/events/stats', params)
}
export function getChampionDialogues(params) {
  return http.get('/api/admin/tuning/feedback/dialogues', params)
}
export function getPromptCandidates(params) {
  return http.get('/api/admin/tuning/prompt/candidates', params)
}
export function updatePromptCandidateStatus(id, data) {
  return http.put(`/api/admin/tuning/prompt/candidates/${id}/status`, data)
}
export function getBanditArms(params) {
  return http.get('/api/admin/tuning/bandit/arms', params)
}
