import { http } from '@/utils/request';

export function getObsConfigList(params = {}) {
  return http.get('/api/obs/config', params)
}

export function getObsConfig(id) {
  return http.get(`/api/obs/config/${id}`)
}

export function createObsConfig(data) {
  return http.post('/api/obs/config', data)
}

export function updateObsConfig(id, data) {
  return http.put(`/api/obs/config/${id}`, data)
}

export function deleteObsConfig(id) {
  return http.delete(`/api/obs/config/${id}`)
}

export function testObsConnection(id) {
  return http.post(`/api/obs/config/${id}/test`)
}

export function setDefaultObsConfig(id) {
  return http.post(`/api/obs/config/${id}/default`)
}

export function getDefaultObsConfig() {
  return http.get('/api/obs/config/default')
}