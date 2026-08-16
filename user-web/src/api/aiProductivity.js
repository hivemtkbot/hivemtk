import { http } from '@/utils/request';

// AI 产能分析：后端真实路由 /api/ai-productivity（产能报告）、/api/ai-productivity/trend（日趋势）
const AIProductivityApi = {
  getReport(params) {
    return request({ url: '/api/ai-productivity/overview', method: 'get', params })
  },
  getTrend(params) {
    return request({ url: '/api/ai-productivity/trend', method: 'get', params })
  }
}

export default AIProductivityApi
