/**
 * 监控告警接入（USR-SY-03）
 * Prometheus exporter + 告警规则
 */

// Prometheus 指标
const PROMETHEUS_METRICS = `
# HELP hivemtk_user_server_requests_total Total HTTP requests
# TYPE hivemtk_user_server_requests_total counter
hivemtk_user_server_requests_total{method="GET",path="/api/customer-sessions",status="200"} 1234

# HELP hivemtk_user_server_request_duration_seconds Request latency
# TYPE hivemtk_user_server_request_duration_seconds histogram
hivemtk_user_server_request_duration_seconds_bucket{le="0.1"} 1000
hivemtk_user_server_request_duration_seconds_bucket{le="0.5"} 1500
hivemtk_user_server_request_duration_seconds_bucket{le="1"} 1700
hivemtk_user_server_request_duration_seconds_bucket{le="+Inf"} 1750

# HELP hivemtk_bridge_pending_messages Pending messages in bridge outbox
# TYPE hivemtk_bridge_pending_messages gauge
hivemtk_bridge_pending_messages{channel="douyin"} 23
hivemtk_bridge_pending_messages{channel="xiaohongshu"} 5

# HELP hivemtk_rag_search_duration_seconds RAG search latency
# TYPE hivemtk_rag_search_duration_seconds histogram
hivemtk_rag_search_duration_seconds_bucket{le="0.5"} 800
hivemtk_rag_search_duration_seconds_bucket{le="1"} 950
hivemtk_rag_search_duration_seconds_bucket{le="2"} 990

# HELP hivemtk_llm_tokens_total LLM tokens consumed
# TYPE hivemtk_llm_tokens_total counter
hivemtk_llm_tokens_total{model="gpt-4",scenario="customer-service"} 1500000
`

// 告警规则
const ALERT_RULES = {
  rules: [
    {
      alert: 'HighErrorRate',
      expr: 'rate(hivemtk_user_server_requests_total{status=~"5.."}[5m]) > 0.05',
      for: '5m',
      annotations: { summary: '错误率 > 5%（5 分钟）', severity: 'critical' }
    },
    {
      alert: 'HighLatency',
      expr: 'histogram_quantile(0.95, hivemtk_user_server_request_duration_seconds) > 1',
      for: '10m',
      annotations: { summary: 'P95 延迟 > 1s（10 分钟）', severity: 'warning' }
    },
    {
      alert: 'BridgeQueueBacklog',
      expr: 'hivemtk_bridge_pending_messages > 1000',
      for: '15m',
      annotations: { summary: '桥接队列堆积 > 1000（15 分钟）', severity: 'warning' }
    },
    {
      alert: 'RAGSearchSlow',
      expr: 'histogram_quantile(0.95, hivemtk_rag_search_duration_seconds) > 2',
      for: '5m',
      annotations: { summary: 'RAG P95 检索 > 2s', severity: 'warning' }
    },
    {
      alert: 'HighCPU',
      expr: 'process_cpu_seconds_total > 0.8',
      for: '10m',
      annotations: { summary: 'CPU 持续 > 80%（10 分钟）', severity: 'warning' }
    }
  ]
}

// 告警通道
export const ALERT_CHANNELS = {
  feishu: (data) => fetch('/api/alerts/feishu', { method: 'POST', body: JSON.stringify(data) }),
  wecom: (data) => fetch('/api/alerts/wecom', { method: 'POST', body: JSON.stringify(data) }),
  email: (data) => fetch('/api/alerts/email', { method: 'POST', body: JSON.stringify(data) }),
  webhook: (url) => (data) => fetch(url, { method: 'POST', body: JSON.stringify(data) })
}

export default { PROMETHEUS_METRICS, ALERT_RULES, ALERT_CHANNELS }
