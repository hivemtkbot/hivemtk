import { http } from '@/utils/request';

// OneID 客户列表
export function listOneID(params) {
  return http.get('/api/customer/oneid/list', params)
}

// 身份冲突列表
export function listConflicts(params) {
  return http.get('/api/customer/oneid/conflicts', params)
}

// 合并两个客户
export function mergeOneID(data) {
  return http.post('/api/customer/oneid/merge', data)
}

// 解决冲突（合并 / 忽略）
export function resolveConflict(id, data) {
  return http.post(`/api/customer/oneid/conflicts/${id}/resolve`, data)
}

// 获取客户身份映射详情
export function getIdentityMappings(customerId) {
  return http.get(`/api/customer-oneid/${customerId}/identities`)
}

// 链接新身份
export function linkIdentity(customerId, data) {
  return http.post(`/api/customer-oneid/${customerId}/identities`, data)
}

// 识别或创建（OneID 解析）
export function resolveIdentity(data) {
  return http.post('/api/customer/oneid/resolve', data)
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
  return http.post('/api/oneid/merge-rules', data)
}
