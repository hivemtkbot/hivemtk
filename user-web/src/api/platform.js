import { http } from '@/utils/request';

export const platformAPI = {
  getLatestMessage() {
    return http.get('/api/platform/message/latest', { _silent: true })
  },

  markMessageRead(messageId) {
    return http.post(`/api/platform/message/${messageId}/read`)
  },

  getLicenseStatus() {
    return http.get('/api/license/status', { _silent: true })
  },

  registerMerchant(data) {
    return http.post('/api/platform/register', data)
  }
};