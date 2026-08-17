import { http } from '@/utils/request';

// 平台相关API
export const platformAPI = {
  // 获取最新消息（后台轮询，平台不可用时静默，避免反复弹错误 toast）
  getLatestMessage() {
    return http.get('/api/platform/message/latest', { _silent: true })
  },

  // 标记消息已读
  markMessageRead(messageId) {
    return http.post(`/api/platform/message/${messageId}/read`)
  },

  // 获取授权状态（开源版无授权概念，返回默认值）
  getLicenseStatus() {
    return http.get('/api/license/status', { _silent: true })
  },

  // 注册商户
  registerMerchant(data) {
    return http.post('/api/platform/register', data)
  }
}