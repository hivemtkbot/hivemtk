import { http } from '@/utils/request'

// 消息中台 MQ API 接口
export const messageHubApi = {
  // 推送消息（标准化）
  pushMessage(data) {
    return http.post('/api/message-hub/push', data)
  },

  // 批量推送
  pushBatch(data) {
    return http.post('/api/message-hub/push-batch', data)
  },

  // 从渠道原始消息推送（自动标准化）
  pushFromChannel(data) {
    return http.post('/api/message-hub/push-from-channel', data)
  },

  // 消息列表
  getMessages(params) {
    return http.get('/api/message-hub/list', params)
  },

  // 消息详情
  getMessageById(id) {
    return http.get(`/api/message-hub/${id}`)
  },

  // 标记已读
  markRead(ids) {
    return http.post(`/api/message-hub/${ids[0]}/read`, { ids })
  },

  // 统计
  getStats(params) {
    return http.get('/api/message-hub/stats', params)
  },

  // 支持的平台与消息类型
  getPlatforms() {
    return http.get('/api/message-hub/platforms')
  }
}
