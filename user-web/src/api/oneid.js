import request from '@/utils/request'

// OneID 客户列表
export function listOneID(params) {
  return request({
    url: '/api/customer/oneid/list',
    method: 'get',
    params
  })
}

// 身份冲突列表
export function listConflicts(params) {
  return request({
    url: '/api/customer/oneid/conflicts',
    method: 'get',
    params
  })
}

// 合并两个客户
export function mergeOneID(data) {
  return request({
    url: '/api/customer/oneid/merge',
    method: 'post',
    data
  })
}

// 解决冲突（合并 / 忽略）
export function resolveConflict(id, data) {
  return request({
    url: `/api/customer/oneid/conflicts/${id}/resolve`,
    method: 'post',
    data
  })
}

// 获取客户身份映射详情
export function getIdentityMappings(customerId) {
  return request({
    url: `/api/customer-oneid/${customerId}/identities`,
    method: 'get'
  })
}

// 链接新身份
export function linkIdentity(customerId, data) {
  return request({
    url: `/api/customer-oneid/${customerId}/identities`,
    method: 'post',
    data
  })
}

// 识别或创建（OneID 解析）
export function resolveIdentity(data) {
  return request({
    url: '/api/customer/oneid/resolve',
    method: 'post',
    data
  })
}
