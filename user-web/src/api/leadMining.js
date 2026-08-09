import request from '@/utils/request'

// 线索发掘配置
export function getLeadMiningConfig() {
  return request({
    url: '/api/lead-mining/config',
    method: 'get'
  })
}

export function saveLeadMiningConfig(data) {
  return request({
    url: '/api/lead-mining/config',
    method: 'post',
    data
  })
}
