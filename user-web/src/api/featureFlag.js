import { http } from '@/utils/request'

/**
 * Feature Flag 独立模块（USR-AI-05）
 * 借鉴：https://github.com/growthbook/growthbook
 *
 * 与 abExperiment 解耦：Feature Flag 关注灰度发布，
 * 实验关注效果对比（需要 conversion events + 统计方法）。
 */

// ===== Flag CRUD =====
export const listFlags = (params) => http.get('/api/feature-flags', params)
export const getFlag = (id) => http.get(`/api/feature-flags/${id}`)
export const createFlag = (data) => http.post('/api/feature-flags', data)
export const updateFlag = (id, data) => http.put(`/api/feature-flags/${id}`, data)
export const deleteFlag = (id) => http.delete(`/api/feature-flags/${id}`)

// ===== Flag 启用/禁用 =====
export const enableFlag = (id) => http.post(`/api/feature-flags/${id}/enable`, {})
export const disableFlag = (id) => http.post(`/api/feature-flags/${id}/disable`, {})

// ===== Flag 灰度发布 =====
export const rolloutFlag = (id, percentage) =>
  http.post(`/api/feature-flags/${id}/rollout`, { percentage })

// ===== Flag 评估（前端用） =====
export const evaluateFlag = (key, attributes = {}) =>
  http.post(`/api/feature-flags/evaluate`, { key, attributes })

// 批量评估（性能优化）
export const evaluateFlags = (keys, attributes = {}) =>
  http.post(`/api/feature-flags/evaluate-batch`, { keys, attributes })

// ===== 审计 + Stale 检测 =====
export const getFlagAudit = (id) => http.get(`/api/feature-flags/${id}/audit`)
export const getStaleFlags = () => http.get('/api/feature-flags/stale')
export const getFlagCodeReferences = (key) =>
  http.get(`/api/feature-flags/${encodeURIComponent(key)}/code-references`)
