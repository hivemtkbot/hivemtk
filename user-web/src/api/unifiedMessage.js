import { http } from '@/utils/request'

export const unifiedMessageApi = {
  getMessages(params) {
    return http.get('/api/unified-messages', params)
  },

  getMessageById(id) {
    return http.get(`/api/unified-messages/${id}`)
  }
};
