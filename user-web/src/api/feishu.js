import { http } from '@/utils/request';

export function listAccounts(params = {}) {
  return http.get('/api/feishu/accounts', params)
}

export function getAccount(id) {
  return http.get(`/api/feishu/accounts/${id}`)
}

export function createAccount(data) {
  return http.post('/api/feishu/accounts', data)
}

export function updateAccount(id, data) {
  return http.put(`/api/feishu/accounts/${id}`, data)
}

export function deleteAccount(id) {
  return http.delete(`/api/feishu/accounts/${id}`)
}

export function testSend(id, data) {
  return http.post(`/api/feishu/accounts/${id}/test-send`, data)
}

export function refreshToken(id) {
  return http.post(`/api/feishu/accounts/${id}/refresh-token`)
}
