import { http } from '@/utils/request';

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
  return http.get(`/api/customer-oneid/${customerId}/identities`)
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

// OneID 统计（总数 / 关联手机号 / 关联邮箱 / 多身份）
export function getOneIDStats() {
  return http.get('/api/customer/oneid/stats')
}

// OPT-UX-04: OneID 合并规则配置
// GET /api/oneid/merge-rules
export function getMergeRules() {
  return http.get('/api/oneid/merge-rules')
}

// POST /api/oneid/merge-rules
export function saveMergeRules(data) {
  return request({
    url: '/api/oneid/merge-rules',
    method: 'post',
    data
  })
}
