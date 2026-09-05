import { http } from '@/utils/request';

export function listChannels(params) {
  return http.get('/api/chat-channels', params)
}

export function getChannel(channelId) {
  return http.get(`/api/chat-channels/${channelId}`)
}

export function createChannel(data) {
  return http.post('/api/chat-channels', data)
}

export function updateChannel(channelId, data) {
  return http.put(`/api/chat-channels/${channelId}`, data)
}

export function deleteChannel(channelId) {
  return http.delete(`/api/chat-channels/${channelId}`)
}

export function rotateAppKey(channelId) {
  return http.post(`/api/chat-channels/${channelId}/rotate-key`)
}

export function resetAppSecret(channelId) {
  return http.post(`/api/chat-channels/${channelId}/reset-secret`)
}
