import { http } from '@/utils/request'

/**
 * 转化漏斗 API
 * 实际后端路由: /api/analytics/funnel 和 /api/analytics/funnel/stage
 */
export const ConversionFunnelApi = {
  // 获取漏斗数据 (含阶段定义和统计)
  getFunnelStages: () => {
    return http.get('/api/analytics/funnel')
  },

  // 保存漏斗阶段 (后端暂无,返回空成功)
  saveFunnelStage: (data) => {
    return Promise.resolve({ code: 'SUCCESS', data: data, message: '漏斗阶段配置仅支持后端' })
  },

  // 更新漏斗阶段 (后端暂无)
  updateFunnelStage: (id, data) => {
    return Promise.resolve({ code: 'SUCCESS', data: { id, ...data }, message: '漏斗阶段配置仅支持后端' })
  },

  // 删除漏斗阶段 (后端暂无)
  deleteFunnelStage: (id) => {
    return Promise.resolve({ code: 'SUCCESS', data: { id, deleted: true }, message: '漏斗阶段配置仅支持后端' })
  },

  // 获取转化率统计
  getFunnelStats: (params) => {
    return http.get('/api/analytics/funnel', params)
  },

  // 各阶段流失分析
  getFunnelLossAnalysis: (params) => {
    return http.get('/api/analytics/funnel/stage', { ...params, analysis: 'loss' })
  },

  // 时间趋势
  getFunnelTrend: (params) => {
    return http.get('/api/analytics/funnel/stage', { ...params, analysis: 'trend' })
  }
}
