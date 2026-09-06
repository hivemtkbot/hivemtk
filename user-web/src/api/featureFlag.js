import { http } from '@/utils/request'

export const listFlags = (params) => http.get('/api/feature-flags', params);
export const getFlag = (id) => http.get(`/api/feature-flags/${id}`)
export const createFlag = (data) => http.post('/api/feature-flags', data)
export const updateFlag = (id, data) => http.put(`/api/feature-flags/${id}`, data)
export const deleteFlag = (id) => http.delete(`/api/feature-flags/${id}`)

export const enableFlag = (id) => http.post(`/api/feature-flags/${id}/enable`, {});
export const disableFlag = (id) => http.post(`/api/feature-flags/${id}/disable`, {})

export const rolloutFlag = (id, percentage) =>
  http.post(`/api/feature-flags/${id}/rollout`, { percentage });

export const evaluateFlag = (key, attributes = {}) =>
  http.post(`/api/feature-flags/evaluate`, { key, attributes });

export const evaluateFlags = (keys, attributes = {}) =>
  http.post(`/api/feature-flags/evaluate-batch`, { keys, attributes });

export const getFlagAudit = (id) => http.get(`/api/feature-flags/${id}/audit`);
export const getStaleFlags = () => http.get('/api/feature-flags/stale')
export const getFlagCodeReferences = (key) =>
  http.get(`/api/feature-flags/${encodeURIComponent(key)}/code-references`)
