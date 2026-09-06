import { http } from '@/utils/request';

export function listAccounts(params = {}) {
  return http.get('/api/telegram/accounts', params)
}

export function getAccount(id) {
  return http.get(`/api/telegram/accounts/${id}`)
}

export function createAccount(data) {
  return http.post('/api/telegram/accounts', data)
}

export function updateAccount(id, data) {
  return http.put(`/api/telegram/accounts/${id}`, data)
}

export function deleteAccount(id) {
  return http.delete(`/api/telegram/accounts/${id}`)
}

export function registerWebhook(id, data = {}) {
  return http.post(`/api/telegram/accounts/${id}/register-webhook`, data)
}

export function testSend(id, data) {
  return http.post(`/api/telegram/accounts/${id}/test-send`, data)
}
