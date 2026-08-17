import { http } from '@/utils/request';

// AI 产能分析：后端真实路由 /api/ai-productivity（产能报告）、/api/ai-productivity/trend（日趋势）
const AIProductivityApi = {
  getReport(params) {
    return http.get('/api/ai-productivity/overview', params)
  },
  getTrend(params) {
    return http.get('/api/ai-productivity/trend', params)
  }
}

export default AIProductivityApi
