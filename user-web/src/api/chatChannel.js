import request from '@/utils/request'

// ============================================================================
// 客服 Web Widget 渠道管理 API
// ----------------------------------------------------------------------------
// 对应后端 /api/chat-channels 路由（B 端 JWT 鉴权）
// ============================================================================

// 渠道列表
export function listChannels(params) {
  return request({
    url: '/api/chat-channels',
    method: 'get',
    params
  })
}

// 渠道详情
export function getChannel(channelId) {
  return request({
    url: `/api/chat-channels/${channelId}`,
    method: 'get'
  })
}

// 创建渠道（返回 AppKey + AppSecret，仅创建时返回一次）
export function createChannel(data) {
  return request({
    url: '/api/chat-channels',
    method: 'post',
    data
  })
}

// 更新渠道
export function updateChannel(channelId, data) {
  return request({
    url: `/api/chat-channels/${channelId}`,
    method: 'put',
    data
  })
}

// 禁用渠道
export function deleteChannel(channelId) {
  return request({
    url: `/api/chat-channels/${channelId}`,
    method: 'delete'
  })
}

// 轮换 AppKey
export function rotateAppKey(channelId) {
  return request({
    url: `/api/chat-channels/${channelId}/rotate-key`,
    method: 'post'
  })
}

// 重置 AppSecret
export function resetAppSecret(channelId) {
  return request({
    url: `/api/chat-channels/${channelId}/reset-secret`,
    method: 'post'
  })
}
