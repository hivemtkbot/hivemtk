import { http } from '@/utils/request';

// 置信度/拟人度/反馈学习 统一管理 API（/api/admin/tuning/*）
// 依据：docs/核心链路优化.md 第十五/十六/十七章

// ============== 置信度 ==============

// 置信度信号列表
export function getConfidenceSignals(params) {
  return http.get('/api/admin/tuning/confidence/signals', params)
}
// 置信度信号详情
export function getConfidenceSignal(id) {
  return http.get(`/api/admin/tuning/confidence/signals/${id}`)
}
// 置信度信号统计
export function getConfidenceSignalStats(params) {
  return http.get('/api/admin/tuning/confidence/signals/stats', params)
}
// 置信度校准列表
export function getConfidenceCalibrations(params) {
  return http.get('/api/admin/tuning/confidence/calibrations', params)
}
// 阈值策略列表
export function getThresholdPolicies(params) {
  return http.get('/api/admin/tuning/confidence/policies', params)
}
// 阈值策略 Upsert（create + update）
export function upsertThresholdPolicy(data) {
  return http.put('/api/admin/tuning/confidence/policies', data)
}

// ============== 拟人度 ==============

// 拟人度评分列表
export function getHumanizeScores(params) {
  return http.get('/api/admin/tuning/humanize/scores', params)
}
// 拟人度评分统计
export function getHumanizeScoreStats(params) {
  return http.get('/api/admin/tuning/humanize/scores/stats', params)
}
// 销冠基线列表
export function getChampionBaselines(params) {
  return http.get('/api/admin/tuning/humanize/baselines', params)
}
// 低质样本列表
export function getLowQualitySamples(params) {
  return http.get('/api/admin/tuning/humanize/low-quality', params)
}

// ============== 反馈学习 ==============

// 反馈事件列表
export function getFeedbackEvents(params) {
  return http.get('/api/admin/tuning/feedback/events', params)
}
// 反馈事件统计
export function getFeedbackEventStats(params) {
  return http.get('/api/admin/tuning/feedback/events/stats', params)
}
// 销冠对话列表
export function getChampionDialogues(params) {
  return http.get('/api/admin/tuning/feedback/dialogues', params)
}
// Prompt 候选列表
export function getPromptCandidates(params) {
  return http.get('/api/admin/tuning/prompt/candidates', params)
}
// 更新 Prompt 候选状态
export function updatePromptCandidateStatus(id, data) {
  return http.put(`/api/admin/tuning/prompt/candidates/${id}/status`, data)
}
// Bandit 臂列表
export function getBanditArms(params) {
  return http.get('/api/admin/tuning/bandit/arms', params)
}
