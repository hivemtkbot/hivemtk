import axios from 'axios'

const publicPost = async (url, data, channelId, visitorId) => {
  return axios.post(url, data, {
    baseURL: window.location.origin,
    headers: {
      'Content-Type': 'application/json',
      'X-Chat-Channel-Id': channelId,
      'X-Chat-Visitor-Id': visitorId
    }
  }).then(res => res.data)
};

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

export const openSession = (data, channelId, visitorId) => {
  return publicPost('/api/chat/public/sessions', data, channelId, visitorId)
};

export const getActiveSession = (channelId, visitorId) => {
  return publicGet('/api/chat/public/sessions/active', {}, channelId, visitorId)
};

export const getRecentClosedSessions = (channelId, visitorId, limit = 10) => {
  return publicGet('/api/chat/public/sessions/recent-closed', { limit }, channelId, visitorId)
};

export const getMessages = (sessionId, page, pageSize, channelId, visitorId) => {
  return publicGet(`/api/chat/public/sessions/${sessionId}/messages`, { page, page_size: pageSize }, channelId, visitorId)
};

export const getOfflineMessages = (sessionId, channelId, visitorId) => {
  return publicGet(`/api/chat/public/sessions/${sessionId}/offline-messages`, {}, channelId, visitorId)
};

export const sendMessage = (sessionId, body, channelId, visitorId) => {
  return publicPost(`/api/chat/public/sessions/${sessionId}/messages`, body, channelId, visitorId)
};

export const requestHumanTransfer = (sessionId, reason, channelId, visitorId) => {
  return publicPost(`/api/chat/public/sessions/${sessionId}/transfer`, { reason }, channelId, visitorId)
};

export const closeSession = (sessionId, channelId, visitorId) => {
  return publicPost(`/api/chat/public/sessions/${sessionId}/close`, {}, channelId, visitorId)
};

export const rateSession = (sessionId, rating, comment, channelId, visitorId) => {
  return publicPost(`/api/chat/public/sessions/${sessionId}/rate`, { rating, comment }, channelId, visitorId)
};

export const getAvailableAgents = (channelId) => {
  return publicGet('/api/chat/public/agents/available', {}, channelId, '')
};

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
