import { http } from '@/utils/http'

// ===== GEO 告警中心（后端 /geo/alerts/*，消费 negative_monitor 等定时任务写入的 geo_alerts） =====
export const listGeoAlerts = (params) =>
  http.get('/api/geo/alerts', params)

export const getGeoAlertsUnreadCount = () =>
  http.get('/api/geo/alerts/unread-count')

export const ackGeoAlert = (id) =>
  http.post(`/api/geo/alerts/${id}/ack`, {})

export const deleteGeoAlert = (id) =>
  http.delete(`/api/geo/alerts/${id}`)
