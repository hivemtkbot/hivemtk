import { http } from '@/utils/request'

// 运维总览（OPT-UX-01）
// 对齐后端真实路由（user-server/internal/router/router.go + monitor/handler.go）：
//   - /api/system/stats      → SystemOpsController.GetSystemStats（CPU/内存/磁盘/用户数/运行时长）
//   - /api/monitor/health     → monitor.HealthOverview（消息收发率/异常数/同步缺口）
//   - /api/monitor/node-health→ monitor.NodeHealthByChannel（各渠道节点健康）
//   - /api/monitor/anomalies  → monitor.Anomalies（异常分组清单）
//   - /api/monitor/alerts/unread → 告警未读数（OpsOverview 原生 fetch，保留）

export function getSystemStatus() {
  return http.get('/api/system/stats')
}

export function getOpsStats() {
  return http.get('/api/monitor/health')
}

export function getModuleStatus() {
  return http.get('/api/monitor/node-health')
}

export function getRecentOperations(limit = 20) {
  return http.get('/api/monitor/anomalies', { limit })
}

// AI 销冠驾驶舱（OPT-UX-02）
// 后端暂无聚合接口 /ai/sales-cockpit；前端改用后端已有的 /api/monitor/health 作为数据源，
// 避免调用不存在的路由触发 NoRoute 兜底 HTML → "非预期响应"。
// 待后端补齐聚合接口后，切回 http.get('/ai/sales-cockpit') 即可。
export function getSalesCockpit() {
  return http.get('/api/monitor/health')
}
