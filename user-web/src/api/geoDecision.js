import { http } from '@/utils/request'

export const getGeoDecisionReport = () => http.get('/api/geo/decision/report');

export const getGeoGapTasks = (limit = 50) =>
  http.get('/api/geo/decision/tasks', { limit })

export const markGeoTaskDone = (id) => http.post(`/api/geo/decision/tasks/${id}/done`)
