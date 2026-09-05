import { http } from '@/utils/request'

export const MonitorApi = {
  traceTree(params) {
    return http.get('/api/monitor/trace-tree', params)
  },
  traces(params) {
    return http.get('/api/monitor/traces', params)
  },
  lifecycle(params) {
    return http.get('/api/monitor/lifecycle', params)
  },
  health() {
    return http.get('/api/monitor/health')
  },
  nodeHealth() {
    return http.get('/api/monitor/node-health')
  },
  latency() {
    return http.get('/api/monitor/latency')
  },
  anomalies() {
    return http.get('/api/monitor/anomalies')
  },
  evalLogs(params) {
    return http.get('/api/monitor/trace-eval/logs', params)
  },
  knowledgeWeights(params) {
    return http.get('/api/monitor/knowledge-weights', params)
  },
  triggerEval(params) {
    return http.post('/api/monitor/trace-eval/trigger', null, { params })
  },

  ragRecallSnapshot() {
    return http.get('/api/rag/recall/snapshot')
  },
  ragRecallSnapshots(params) {
    return http.get('/api/rag/recall/snapshots', params)
  },
  ragRecallCollect(params) {
    return http.post('/api/rag/recall/collect', params)
  },
  ragRecallStart() {
    return http.post('/api/rag/recall/start')
  },
  ragRecallStop() {
    return http.post('/api/rag/recall/stop')
  },
  ragRecallMetrics(params) {
    return http.get('/api/rag/recall/metrics', params)
  },
  ragLowRecallQueries(params) {
    return http.get('/api/rag/recall/low-recall', params)
  }
};
