import { http } from '@/utils/request'

// 全链路业务追踪监控接口（user-server /api/monitor/*）
// 这些端点挂 InitGuard 私域鉴权，前端直接调用即可返回数据（http 拦截器已解包为内层 data）。

export const MonitorApi = {
  // 任意一条消息 / 单轮对话的完整链路明细（生命周期节点 + agent 多轮 + 多工具）
  // 优先级：trace_id > msg_id（反查）> conversation_id（最近一轮）
  traceTree(params) {
    return http.get('/api/monitor/trace-tree', params)
  },
  // 链路会话列表（最近 N 轮）
  traces(params) {
    return http.get('/api/monitor/traces', params)
  },
  // 还原某会话 / 某轮的业务链路节点
  lifecycle(params) {
    return http.get('/api/monitor/lifecycle', params)
  },
  // 业务健康概览（入站/出站速率、待发、送达、异常等）
  health() {
    return http.get('/api/monitor/health')
  },
  // 按渠道 × 节点聚合：响应时间(avg/p95) + 异常率
  nodeHealth() {
    return http.get('/api/monitor/node-health')
  },
  // 按渠道端到端时延（上报接入 → 送达确认）
  latency() {
    return http.get('/api/monitor/latency')
  },
  // 异常聚合（数据缺口 / 卡住可达 / 卡住不可达 / 不可达 / 节点异常）
  anomalies() {
    return http.get('/api/monitor/anomalies')
  },
  // 追踪自学习：最近打分审计记录（LLM 对每条 trace 的打分 + 权重调整明细）
  evalLogs(params) {
    return http.get('/api/monitor/trace-eval/logs', params)
  },
  // 追踪自学习：知识库权重排行（权重偏离 1.0 最大的 chunk）
  knowledgeWeights(params) {
    return http.get('/api/monitor/knowledge-weights', params)
  },
  // 追踪自学习：手动触发评估（扫描最近 hours 小时内未评估的 trace 批量打分+调权）
  triggerEval(params) {
    return http.post('/api/monitor/trace-eval/trigger', null, { params })
  },

  // ============ RAG 召回质量监控 ============
  // 获取最新召回率监控快照
  ragRecallSnapshot() {
    return http.get('/api/rag/recall/snapshot')
  },
  // 列出最近 N 条监控快照
  ragRecallSnapshots(params) {
    return http.get('/api/rag/recall/snapshots', params)
  },
  // 手动触发一次召回率指标采集
  ragRecallCollect(params) {
    return http.post('/api/rag/recall/collect', params)
  },
  // 启动后台定时采集
  ragRecallStart() {
    return http.post('/api/rag/recall/start')
  },
  // 停止后台定时采集
  ragRecallStop() {
    return http.post('/api/rag/recall/stop')
  },
  // 查询时间窗口内的召回指标聚合
  ragRecallMetrics(params) {
    return http.get('/api/rag/recall/metrics', params)
  },
  // 查询低召回样本
  ragLowRecallQueries(params) {
    return http.get('/api/rag/recall/low-recall', params)
  }
}
