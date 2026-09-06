import { http } from '@/utils/request'

export const shortLinkApi = {
  getList(params) {
    return http.get('/api/shortlink/list', { params })
  },
  
  getById(id) {
    return http.get(`/api/shortlink/${id}`)
  },
  
  create(data) {
    return http.post('/api/shortlink/create', data)
  },
  
  update(data) {
    return http.put('/api/shortlink/update', data)
  },
  
  delete(id) {
    return http.delete(`/api/shortlink/delete/${id}`)
  },
  
  access(data) {
    return http.post('/api/shortlink/access', data)
  },
  
  generateShortCode(data) {
    return http.post('/api/shortlink/generate', data)
  },
  
  getStats(id, params) {
    return http.get(`/api/shortlink/${id}/stats`, { params })
  },
  
  getAllStats(params) {
    return http.get('/api/shortlink/stats/all', { params })
  },
  
  share(id, data) {
    return http.post(`/api/shortlink/${id}/share`, data)
  }
};