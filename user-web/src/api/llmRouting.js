import { http } from '@/utils/request'

/**
 * LLM 多模型路由 API
 *
 * 端点对齐（2026-07-24 补全）：
 *  - GET    /api/llm/models            列出所有 provider
 *  - GET    /api/llm/models/:name      单个 provider
 *  - POST   /api/llm/models            新增 provider
 *  - PUT    /api/llm/models/:name      更新 provider
 *  - DELETE /api/llm/models/:name      删除 provider
 *  - POST   /api/llm/models/:name/test 测试 provider 连通性
 *  - GET    /api/llm/strategies        列出场景路由
 *  - PUT    /api/llm/strategies        批量更新场景路由
 *  - GET    /api/llm/scenarios         场景列表（strategies 别名）
 *  - GET    /api/llm/scene-routing     列出场景路由（兼容旧前端）
 *  - PUT    /api/llm/scene-routing     批量更新场景路由（兼容旧前端）
 *  - GET    /api/llm/fallback          Fallback 兜底配置
 *  - GET    /api/llm/audit             路由变更审计
 *  - GET    /api/llm/stats             进程内实时统计
 *  - GET    /api/llm/usage             跨进程历史统计
 *  - GET    /api/llm/cost-stats        成本统计（含按场景维度）
 *  - GET    /api/llm/health            整体健康度（含熔断器）
 *  - GET    /api/llm/model-type-stats  本地/云端分类统计
 *  - GET    /api/llm/egress-audit      出域审计
 *  - GET    /api/llm/egress-alerts     出域告警
 */
export const LlmRoutingApi = {
  // ===== Provider / Model 管理 =====

  // 获取模型列表
  getModelList: (params) => {
    return http.get('/api/llm/models', params)
  },

  // 获取模型详情
  getModelDetail: (name) => {
    return http.get(`/api/llm/models/${encodeURIComponent(name)}`)
  },

  // 新增模型
  createModel: (data) => {
    return http.post('/api/llm/models', data)
  },

  // 更新模型（按 provider name）
  updateModel: (name, data) => {
    return http.put(`/api/llm/models/${encodeURIComponent(name)}`, data)
  },

  // 删除模型
  deleteModel: (name) => {
    return http.delete(`/api/llm/models/${encodeURIComponent(name)}`)
  },

  // 更新模型状态（启用/禁用）
  updateModelStatus: (name, status) => {
    return http.put(`/api/llm/models/${encodeURIComponent(name)}`, { enabled: status })
  },

  // 测试模型连通性
  // name: provider name; body: { prompt, timeout_seconds }
  testModel: (name, body = {}) => {
    return http.post(`/api/llm/models/${encodeURIComponent(name)}/test`, body)
  },

  // ===== 场景路由管理 =====

  // 获取场景路由配置
  getSceneRouting: () => {
    return http.get('/api/llm/scene-routing')
  },

  // 获取场景列表（与 strategies 同义）
  getScenarios: () => {
    return http.get('/api/llm/scenarios')
  },

  // 保存场景路由配置（批量更新）
  // data 应为 { routes: [...] } 或直接数组
  saveSceneRouting: (data) => {
    const payload = Array.isArray(data) ? { routes: data } : data
    return http.put('/api/llm/scene-routing', payload)
  },

  // ===== Fallback 策略 =====

  // 获取 Fallback 兜底配置
  getFallbackStrategy: () => {
    return http.get('/api/llm/fallback')
  },

  // ===== 路由审计 =====

  // 获取路由变更审计历史
  // scenario: 可选场景过滤；limit: 返回条数
  getAuditHistory: (scenario, limit = 50) => {
    const params = { limit }
    if (scenario) params.scenario = scenario
    return http.get('/api/llm/audit', params)
  },

  // ===== 用量统计 =====

  // 进程内实时 provider 维度统计
  getStats: () => {
    return http.get('/api/llm/stats')
  },

  // 跨进程历史统计（按 window）
  getUsage: (window = 'all') => {
    return http.get('/api/llm/usage', { window })
  },

  // 成本统计（含按场景维度，含 by_model 字段别名）
  getCostStats: (window = 'month') => {
    return http.get('/api/llm/cost-stats', { window })
  },

  // ===== 健康度 =====

  // 整体健康度（含熔断器、错误计数、最后错误）
  getHealth: () => {
    return http.get('/api/llm/health')
  },

  // ===== 本地/云端分类统计与出域审计 =====

  // 本地 vs 云端分类统计（按 model_type 维度）
  getModelTypeStats: (window = 'month') => {
    return http.get('/api/llm/model-type-stats', { window })
  },

  // 出域审计（检测"应该本地但走云端"的异常调用）
  getEgressAudit: (params = {}) => {
    return http.get('/api/llm/egress-audit', params)
  },

  // 出域告警
  getEgressAlerts: (params = {}) => {
    return http.get('/api/llm/egress-alerts', params)
  }
}
