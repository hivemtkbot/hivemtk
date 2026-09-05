import { http } from '@/utils/request';

const AIProductivityApi = {
  getReport(params) {
    return http.get('/api/ai-productivity/overview', params)
  },
  getTrend(params) {
    return http.get('/api/ai-productivity/trend', params)
  }
};

export default AIProductivityApi
