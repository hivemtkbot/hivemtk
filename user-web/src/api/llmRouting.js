import { http } from '@/utils/request'

/**
 * LLM 多模型路由 API
 * 后端暂无专门路由，复用 /api/system/config 与 /api/app-config，
 * 并预留 /api/llm/* 路径，后端接口缺失时页面显示空状态不报错。
 */
export const LlmRoutingApi = {
  // 获取模型列表（模型名、厂商、状态、优先级、配额）
  getModelList: (params) => {
    return http.get('/api/llm/models', params)
  },

  // 获取模型详情
  getModelDetail: (id) => {
    return http.get(`/api/llm/models/${id}`)
  },

  // 新增/更新模型配置
  saveModel: (data) => {
    return http.post('/api/llm/models', data)
  },

  // 删除模型
  deleteModel: (id) => {
    return http.delete(`/api/llm/models/${id}`)
  },

  // 更新模型状态（启用/禁用）
  updateModelStatus: (id, status) => {
    return http.put(`/api/llm/models/${id}/status`, { status })
  },

  // 获取场景路由配置（意图 → 模型映射）
  getSceneRouting: () => {
    return http.get('/api/llm/scene-routing')
  },

  // 保存场景路由配置
  saveSceneRouting: (data) => {
    return http.put('/api/llm/scene-routing', data)
  },

  // 获取 Fallback 策略
  getFallbackStrategy: () => {
    return http.get('/api/llm/fallback')
  },

  // 保存 Fallback 策略
  saveFallbackStrategy: (data) => {
    return http.put('/api/llm/fallback', data)
  },

  // 获取成本统计
  getCostStats: (params) => {
    return http.get('/api/llm/cost-stats', params)
  },

  // 测试模型连通性
  testModel: (id) => {
    return http.post(`/api/llm/models/${id}/test`)
  }
}
