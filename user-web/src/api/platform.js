import request from '@/utils/request'

// 平台相关API
export const platformAPI = {
  // 获取最新消息（后台轮询，平台不可用时静默，避免反复弹错误 toast）
  getLatestMessage() {
    return request({
      url: '/api/platform/message/latest',
      method: 'get',
      _silent: true
    })
  },

  // 标记消息已读
  markMessageRead(messageId) {
    return request({
      url: `/api/platform/message/${messageId}/read`,
      method: 'post'
    })
  },

  // 获取授权状态 - 实际后端路由为 /api/license/status（开源自部署，无 OTA 授权）
  getLicenseStatus() {
    return request({
      url: '/api/license/status',
      method: 'get'
    })
  },

  // 上报API日志
  reportAPILog(data) {
    return request({
      url: '/api/platform/report-api-log',
      method: 'post',
      data
    })
  },

  // 注册商户
  registerMerchant(data) {
    return request({
      url: '/api/platform/register',
      method: 'post',
      data
    })
  }
}