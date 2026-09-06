import { http } from '@/utils/request'

export const submitCSAT = (sessionId, data) =>
  http.post(`/api/customer-sessions/${sessionId}/csat`, data);

export const getCSATStats = (params) =>
  http.get('/api/csat/stats', params);

export const getCSATTrend = (params) =>
  http.get('/api/csat/trend', params);

export const getNegativeCSAT = (params) =>
  http.get('/api/csat/negative', params);

export const triggerCSATSurvey = (sessionId) =>
  http.post(`/api/customer-sessions/${sessionId}/csat/trigger`, {});

export const getCSATTemplate = () => http.get('/api/csat/template');
export const saveCSATTemplate = (data) => http.put('/api/csat/template', data)
