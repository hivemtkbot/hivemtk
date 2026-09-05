import { http } from '@/utils/request';

const ConversionFunnelApi = {
  getFunnel(params) {
    return http.get('/api/conversion-funnels', params)
  },
  getStageDetails(stage, params) {
    return http.get('/api/conversion-funnels/stage', { params: { stage, ...params } })
  }
};

export default ConversionFunnelApi
