import { http } from '@/utils/request'

// 统一消息 API 接口
export const unifiedMessageApi = {
  // 获取消息列表
  getMessages(params) {
    return http.get('/api/unified-messages', params)
  },

  // 获取消息详情
  getMessageById(id) {
    return http.get(`/api/unified-messages/${id}`)
  },

  // 获取消息回复列表
  getReplies(id, params) {
    return http.get(`/api/unified-messages/${id}/replies`, params)
  }
}
