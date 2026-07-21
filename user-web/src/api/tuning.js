import request from '@/utils/request'

// 置信度/拟人度/反馈学习 统一管理 API（/api/admin/tuning/*）
// 依据：docs/核心链路优化.md 第十五/十六/十七章

// ============== 置信度 ==============

// 置信度信号列表
export function getConfidenceSignals(params) {
  return request({ url: '/api/admin/tuning/confidence/signals', method: 'get', params })
}
// 置信度信号详情
export function getConfidenceSignal(id) {
  return request({ url: `/api/admin/tuning/confidence/signals/${id}`, method: 'get' })
}
// 置信度信号统计
export function getConfidenceSignalStats(params) {
  return request({ url: '/api/admin/tuning/confidence/signals/stats', method: 'get', params })
}
// 置信度校准列表
export function getConfidenceCalibrations(params) {
  return request({ url: '/api/admin/tuning/confidence/calibrations', method: 'get', params })
}
// 阈值策略列表
export function getThresholdPolicies(params) {
  return request({ url: '/api/admin/tuning/confidence/policies', method: 'get', params })
}
// 阈值策略 Upsert（create + update）
export function upsertThresholdPolicy(data) {
  return request({ url: '/api/admin/tuning/confidence/policies', method: 'put', data })
}

// ============== 拟人度 ==============

// 拟人度评分列表
export function getHumanizeScores(params) {
  return request({ url: '/api/admin/tuning/humanize/scores', method: 'get', params })
}
// 拟人度评分统计
export function getHumanizeScoreStats(params) {
  return request({ url: '/api/admin/tuning/humanize/scores/stats', method: 'get', params })
}
// 销冠基线列表
export function getChampionBaselines(params) {
  return request({ url: '/api/admin/tuning/humanize/baselines', method: 'get', params })
}
// 低质样本列表
export function getLowQualitySamples(params) {
  return request({ url: '/api/admin/tuning/humanize/low-quality', method: 'get', params })
}

// ============== 反馈学习 ==============

// 反馈事件列表
export function getFeedbackEvents(params) {
  return request({ url: '/api/admin/tuning/feedback/events', method: 'get', params })
}
// 反馈事件统计
export function getFeedbackEventStats(params) {
  return request({ url: '/api/admin/tuning/feedback/events/stats', method: 'get', params })
}
// 销冠对话列表
export function getChampionDialogues(params) {
  return request({ url: '/api/admin/tuning/feedback/dialogues', method: 'get', params })
}
// Prompt 候选列表
export function getPromptCandidates(params) {
  return request({ url: '/api/admin/tuning/prompt/candidates', method: 'get', params })
}
// 更新 Prompt 候选状态
export function updatePromptCandidateStatus(id, data) {
  return request({ url: `/api/admin/tuning/prompt/candidates/${id}/status`, method: 'put', data })
}
// Bandit 臂列表
export function getBanditArms(params) {
  return request({ url: '/api/admin/tuning/bandit/arms', method: 'get', params })
}
