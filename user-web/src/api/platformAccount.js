import { http } from '@/utils/request'

export const platformAccountApi = {
  getAccounts(params) {
    return http.get('/api/platform-accounts', { params })
  },

  getPlatforms() {
    return http.get('/api/platform-accounts/platforms')
  },

  getAccountById(id) {
    return http.get(`/api/platform-accounts/${id}`)
  },

  createAccount(data) {
    return http.post('/api/platform-accounts', data)
  },

  updateAccount(id, data) {
    return http.put(`/api/platform-accounts/${id}`, data)
  },

  deleteAccount(id) {
    return http.delete(`/api/platform-accounts/${id}`)
  },

  loginAccount(id, data) {
    return http.post(`/api/platform-accounts/${id}/login`, data)
  },

  checkStatus(id) {
    return http.get(`/api/platform-accounts/${id}/status`)
  }
};
