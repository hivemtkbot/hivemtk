import axios from 'axios'

// ============================================================================
// 公开 chat API 封装（访客端，无 JWT）
// ----------------------------------------------------------------------------
// 私域部署模式（2026-07-17 优化）：
//   - 不强制要求 AppKey
//   - channelId 通过 X-Chat-Channel-Id Header 传递（缺失时后端使用 default）
//   - visitorId 通过 X-Chat-Visitor-Id Header 传递（必须，用于会话归属）
// ============================================================================

const publicPost = async (url, data, channelId, visitorId) => {
  return axios.post(url, data, {
    baseURL: window.location.origin,
    headers: {
      'Content-Type': 'application/json',
      'X-Chat-Channel-Id': channelId,
      'X-Chat-Visitor-Id': visitorId
    }
  }).then(res => res.data)
}

const publicGet = async (url, params, channelId, visitorId) => {
  return axios.get(url, {
    baseURL: window.location.origin,
    params,
    headers: {
      'X-Chat-Channel-Id': channelId,
      'X-Chat-Visitor-Id': visitorId
    }
  }).then(res => res.data)
}

// 打开会话
export const openSession = (data, channelId, visitorId) => {
  return publicPost('/api/chat/public/sessions', data, channelId, visitorId)
}

// 获取活跃会话
export const getActiveSession = (channelId, visitorId) => {
  return publicGet('/api/chat/public/sessions/active', {}, channelId, visitorId)
}

// 获取最近已结束会话
export const getRecentClosedSessions = (channelId, visitorId, limit = 10) => {
  return publicGet('/api/chat/public/sessions/recent-closed', { limit }, channelId, visitorId)
}

// 获取历史消息
export const getMessages = (sessionId, page, pageSize, channelId, visitorId) => {
  return publicGet(`/api/chat/public/sessions/${sessionId}/messages`, { page, page_size: pageSize }, channelId, visitorId)
}

// 拉取离线消息
export const getOfflineMessages = (sessionId, channelId, visitorId) => {
  return publicGet(`/api/chat/public/sessions/${sessionId}/offline-messages`, {}, channelId, visitorId)
}

// 发送消息
export const sendMessage = (sessionId, body, channelId, visitorId) => {
  return publicPost(`/api/chat/public/sessions/${sessionId}/messages`, body, channelId, visitorId)
}

// 转人工
export const requestHumanTransfer = (sessionId, reason, channelId, visitorId) => {
  return publicPost(`/api/chat/public/sessions/${sessionId}/transfer`, { reason }, channelId, visitorId)
}

// 关闭会话
export const closeSession = (sessionId, channelId, visitorId) => {
  return publicPost(`/api/chat/public/sessions/${sessionId}/close`, {}, channelId, visitorId)
}

// 评分
export const rateSession = (sessionId, rating, comment, channelId, visitorId) => {
  return publicPost(`/api/chat/public/sessions/${sessionId}/rate`, { rating, comment }, channelId, visitorId)
}

// 可用坐席数
export const getAvailableAgents = (channelId) => {
  return publicGet('/api/chat/public/agents/available', {}, channelId, '')
}

export default {
  openSession,
  getActiveSession,
  getRecentClosedSessions,
  getMessages,
  getOfflineMessages,
  sendMessage,
  requestHumanTransfer,
  closeSession,
  rateSession,
  getAvailableAgents
}
