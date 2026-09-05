import { http } from '@/utils/request'

export const messageHubApi = {
  pushMessage(data) {
    return http.post('/api/message-hub/push', data)
  },

  pushBatch(data) {
    return http.post('/api/message-hub/push-batch', data)
  },

  pushFromChannel(data) {
    return http.post('/api/message-hub/push-from-channel', data)
  },

  getMessages(params) {
    return http.get('/api/message-hub/list', params)
  },

  getMessageById(id) {
    return http.get(`/api/message-hub/${id}`)
  },

  markRead(ids) {
    return http.post(`/api/message-hub/${ids[0]}/read`, { ids })
  },

  getStats(params) {
    return http.get('/api/message-hub/stats', params)
  },

  getPlatforms() {
    return http.get('/api/message-hub/platforms')
  }
};
