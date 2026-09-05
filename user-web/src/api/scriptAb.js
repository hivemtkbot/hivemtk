import { http } from '@/utils/request';

export const scriptAbApi = {
  listVersions: (scriptId) => http.get(`/api/script-library/${scriptId}/versions`),
  createVersion: (scriptId, data) => http.post(`/api/script-library/${scriptId}/versions`, data),
  activateVersion: (scriptId, versionId) =>
    http.put(`/api/script-library/${scriptId}/versions/${versionId}/activate`),
  expireScript: (scriptId, data) => http.post(`/api/script-library/${scriptId}/expire`, data),
  getAbStats: (scriptId, params) => http.get(`/api/script-library/${scriptId}/ab-stats`, params),
  updateAbConfig: (scriptId, data) => http.put(`/api/script-library/${scriptId}/ab-config`, data),
  recordConversion: (data) => http.post('/api/script-ab/conversion', data),
};
