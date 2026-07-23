import request from '@/utils/request'

// 转化漏斗：后端实际路由 /api/conversion-funnels（GET 漏斗报告）、/api/conversion-funnels/stage（阶段详情）
const ConversionFunnelApi = {
  // 漏斗报告（含各阶段 count/rate/drop_rate 与端到端 conversion/total）
  getFunnel(params) {
    return request({ url: '/api/conversion-funnels', method: 'get', params })
  },
  // 阶段详情（含 count/rate/avg_duration_seconds/top_sources）
  getStageDetails(stage, params) {
    return request({ url: '/api/conversion-funnels/stage', method: 'get', params: { stage, ...params } })
  }
}

export default ConversionFunnelApi
