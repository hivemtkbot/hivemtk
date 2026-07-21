import request from '@/utils/request'

/**
 * 获取授权状态信息
 */
export function getLicenseStatus() {
  return request({
    url: '/api/license/status',
    method: 'get'
  })
}