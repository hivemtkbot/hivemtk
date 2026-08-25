import { http } from '@/utils/request'

// GEO 决策链 L4 报表（v3 度量重构：结果指标置顶）
export const getGeoDecisionReport = () => http.get('/api/geo/decision/report')

export const getGeoGapTasks = (limit = 50) =>
  http.get('/api/geo/decision/tasks', { limit })

export const markGeoTaskDone = (id) => http.post(`/api/geo/decision/tasks/${id}/done`)
