import { http } from '@/utils/request'

export const faqApi = {
  list(params) {
    return http.get('/api/faqs', params)
  },

  get(id) {
    return http.get(`/api/faqs/${id}`)
  },

  create(data) {
    return http.post('/api/faqs', data)
  },

  update(id, data) {
    return http.put(`/api/faqs/${id}`, data)
  },

  remove(id) {
    return http.delete(`/api/faqs/${id}`)
  },

  match(data) {
    return http.post('/api/faqs/match', data)
  },

  stats() {
    return http.get('/api/faqs/stats')
  }
};
