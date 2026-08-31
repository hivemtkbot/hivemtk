import { http } from '@/utils/http'

// GEO 调度器状态与手动触发
export const getSchedulerStatus = () =>
  http.get('/api/geo/scheduler/status')

export const runSchedulerTask = (task) =>
  http.post(`/api/geo/scheduler/run/${task}`)
