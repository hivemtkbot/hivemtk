import { http } from '@/utils/request'

export const wecomAccountApi = {
  listAccounts() {
    return http.get('/api/wecom/health/accounts')
  },

  listRiskAccounts() {
    return http.get('/api/wecom/health/accounts/risks')
  },

  selectHealthyAccount() {
    return http.get('/api/wecom/health/accounts/select')
  },

  getHealthSummary() {
    return http.get('/api/wecom/health/accounts/summary')
  },

  getLatestHealth(id) {
    return http.get(`/api/wecom/health/accounts/${id}`)
  },

  listHealthHistory(id, params) {
    return http.get(`/api/wecom/health/accounts/${id}/history`, params)
  },

  reportHealth(id, data) {
    return http.post(`/api/wecom/health/accounts/${id}`, data)
  },

  updateAccountStatus(id, data) {
    return http.post(`/api/wecom/health/accounts/${id}/status`, data)
  },

  consumeQuota(id, count) {
    return http.post(`/api/wecom/health/accounts/${id}/quota/consume`, { count })
  },

  resetDailyQuota() {
    return http.post('/api/wecom/health/accounts/quota/reset')
  },

  ingestMessage(data) {
    return http.post('/api/wecom/messages/ingest', data)
  },

  sendMessage(data) {
    return http.post('/api/wecom/messages/send', data)
  },

  createAccount(data) {
    return http.post('/api/wecom/accounts', data)
  },

  updateAccount(id, data) {
    return http.put(`/api/wecom/accounts/${id}`, data)
  },

  deleteAccount(id) {
    return http.delete(`/api/wecom/accounts/${id}`)
  },

  getCustomers(params) {
    return http.get('/api/wecom/customers', { params })
  },

  getGroups(params) {
    return http.get('/api/wecom/groups', { params })
  },

  getTags() {
    return http.get('/api/wecom/tags')
  },

  getMessages(params) {
    return http.get('/api/wecom/messages', { params })
  },

  syncCustomers(id) {
    return http.post(`/api/wecom/accounts/${id}/sync-customers`)
  },

  syncGroups(id) {
    return http.post(`/api/wecom/accounts/${id}/sync-groups`)
  },

  syncTags(id) {
    return http.post(`/api/wecom/accounts/${id}/sync-tags`)
  },

  refreshAccount(id) {
    return http.post(`/api/wecom/accounts/${id}/refresh`)
  },

  sendMessageById(id, data) {
    return http.post(`/api/wecom/accounts/${id}/send-message`, data)
  }
};
