import { http } from '@/utils/request'

export const sendCollaborativeMessage = (sessionId, data) =>
  http.post(`/api/customer-sessions/${sessionId}/messages`, data);

export const addInternalNote = (sessionId, data) =>
  http.post(`/api/customer-sessions/${sessionId}/internal-notes`, data);

export const searchMentionUsers = (params) =>
  http.get('/api/users/search', params);

export const markMentionRead = (mentionId) =>
  http.post(`/api/mentions/${mentionId}/read`, {});

export const getMyMentions = (params) =>
  http.get('/api/mentions/mine', params);

export const acquireEditLock = (sessionId) =>
  http.post(`/api/customer-sessions/${sessionId}/edit-lock`, { holder: 'me' }, { _silent: true });

export const releaseEditLock = (sessionId) =>
  http.delete(`/api/customer-sessions/${sessionId}/edit-lock`, { _silent: true });

export const getEditLock = (sessionId) =>
  http.get(`/api/customer-sessions/${sessionId}/edit-lock`, { _silent: true });

export const listInternalNotes = (sessionId, params) =>
  http.get(`/api/customer-sessions/${sessionId}/internal-notes`, params);
