import { http } from '@/utils/request'

/**
 * CSAT 满意度调查（USR-WB-05）
 * 借鉴：libredesk CSAT
 *
 * 流程：会话关闭 → 1h 后触发 → 通过触达通道发送调查 → 客户评分 → 数据回流
 */

// CSAT 评分提交
export const submitCSAT = (sessionId, data) =>
  http.post(`/api/customer-sessions/${sessionId}/csat`, data)

// 获取 CSAT 统计
export const getCSATStats = (params) =>
  http.get('/api/csat/stats', params)

// CSAT 趋势（按日/周/月）
export const getCSATTrend = (params) =>
  http.get('/api/csat/trend', params)

// CSAT 差评列表
export const getNegativeCSAT = (params) =>
  http.get('/api/csat/negative', params)

// 触发 CSAT 调查（手动 / 自动）
export const triggerCSATSurvey = (sessionId) =>
  http.post(`/api/customer-sessions/${sessionId}/csat/trigger`, {})

// CSAT 模板配置
export const getCSATTemplate = () => http.get('/api/csat/template')
export const saveCSATTemplate = (data) => http.put('/api/csat/template', data)
