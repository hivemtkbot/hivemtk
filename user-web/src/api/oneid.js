import { http } from '@/utils/request';

export function listOneID(params) {
  return http.get('/api/customer/oneid/list', params)
}

export function listConflicts(params) {
  return http.get('/api/customer/oneid/conflicts', params)
}

export function mergeOneID(data) {
  return http.post('/api/customer/oneid/merge', data)
}

export function resolveConflict(id, data) {
  return http.post(`/api/customer/oneid/conflicts/${id}/resolve`, data)
}

export function getIdentityMappings(customerId) {
  return http.get(`/api/customer-oneid/${customerId}/identities`)
}

export function linkIdentity(customerId, data) {
  return http.post(`/api/customer-oneid/${customerId}/identities`, data)
}

export function resolveIdentity(data) {
  return http.post('/api/customer/oneid/resolve', data)
}

export function getOneIDStats() {
  return http.get('/api/customer/oneid/stats')
}

export function getMergeRules() {
  return http.get('/api/oneid/merge-rules')
}

export function saveMergeRules(data) {
  return http.post('/api/oneid/merge-rules', data)
}
