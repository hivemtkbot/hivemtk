import { http } from '@/utils/request'

/**
 * 数据大屏预置模板（USR-AN-03）
 */

export const listDashboardTemplates = () => http.get('/api/dashboard-templates')
export const getDashboardTemplate = (id) => http.get(`/api/dashboard-templates/${id}`)
export const cloneTemplate = (id, name) =>
  http.post(`/api/dashboard-templates/${id}/clone`, { name })
export const publishTemplate = (id, data) =>
  http.post(`/api/dashboard-templates/${id}/publish`, data)
export const rateTemplate = (id, data) =>
  http.post(`/api/dashboard-templates/${id}/rate`, data)

// 5+ 预置模板
export const PRESET_TEMPLATES = [
  { id: 'sales-cockpit', name: 'AI 销冠驾驶舱', category: 'sales', description: '智能体效果、转化、响应' },
  { id: 'channel-roi', name: '渠道转化', category: 'reach', description: '5 平台卡片 ROI' },
  { id: 'customer-health', name: '客户健康度', category: 'crm', description: 'RFM + OneID + 流失' },
  { id: 'reach-effectiveness', name: '触达效果', category: 'reach', description: '邮件/短信送达+点击' },
  { id: 'sla-board', name: 'SLA 看板', category: 'cs', description: '首响/解决达标率' }
]
