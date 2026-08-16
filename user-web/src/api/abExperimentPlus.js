import { http } from '@/utils/request'

/**
 * A/B 实验 + MAB 增强（USR-AN-05）
 * 借鉴：GrowthBook + Unleash
 * 统计方法扩展：Bayesian / Frequentist / Sequential / CUPED
 */

export const getExperimentWithStats = (id, method = 'frequentist') =>
  http.get(`/api/ab-experiments/${id}/stats`, { method })

export const getExperimentDiagnostics = (id) =>
  http.get(`/api/ab-experiments/${id}/diagnostics`)

// CUPED（Controlled-experiment Using Pre-Experiment Data）
// 减少方差：使用实验前数据协变量调整
export const getExperimentWithCUPED = (id) =>
  http.get(`/api/ab-experiments/${id}/cuped`)

// Sequential Test（序贯检验，可提前停止）
export const sequentialTest = (id, alpha = 0.05) =>
  http.post(`/api/ab-experiments/${id}/sequential-test`, { alpha })

// Bayesian
export const bayesianTest = (id) =>
  http.post(`/api/ab-experiments/${id}/bayesian-test`, {})

// Feature Evaluation Diagnostics（开发调试）
export const getFeatureEvalLog = (key, userId) =>
  http.get(`/api/feature-flags/${encodeURIComponent(key)}/eval-log`, { user_id: userId })
