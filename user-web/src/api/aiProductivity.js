import { http } from '@/utils/request'

/**
 * AI 产能分析 API
 * 实际后端路由: /api/analytics/ai-productivity/*
 */
export const AiProductivityApi = {
  // 概览数据
  getOverview: (params) => {
    return http.get('/api/analytics/ai-productivity', params)
  },

  // 对话量统计
  getConversationStats: (params) => {
    return http.get('/api/analytics/ai-productivity', { ...params, metric: 'conversations' })
  },

  // 转化率分析
  getConversionRate: (params) => {
    return http.get('/api/analytics/ai-productivity', { ...params, metric: 'conversion_rate' })
  },

  // 响应时长分析
  getResponseTimeStats: (params) => {
    return http.get('/api/analytics/ai-productivity', { ...params, metric: 'response_time' })
  },

  // 销冠能力画像
  getTopSalesPortrait: (params) => {
    return http.get('/api/analytics/ai-productivity', { ...params, metric: 'top_sales' })
  },

  // 客服/坐席产能排行
  getAgentRanking: (params) => {
    return http.get('/api/analytics/ai-productivity', { ...params, metric: 'agent_ranking' })
  }
}
