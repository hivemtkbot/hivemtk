import { http } from '@/utils/request'

export const sopTemplateApi = {
  list(params) {
    return http.get('/api/sop-templates', params)
  },

  get(id) {
    return http.get(`/api/sop-templates/${id}`)
  },

  create(data) {
    return http.post('/api/sop-templates', data)
  },

  update(id, data) {
    return http.put(`/api/sop-templates/${id}`, data)
  },

  remove(id) {
    return http.delete(`/api/sop-templates/${id}`)
  },

  match(data) {
    return http.post('/api/sop-templates/match', data)
  }
};
