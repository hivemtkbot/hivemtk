import { http } from '@/utils/request'

export const usersApi = {
  list: (params) => {
    return http.get('/api/users', { params })
  },
  
  get: (id) => {
    return http.get(`/api/users/${id}`)
  },
  
  create: (data) => {
    return http.post('/api/users', data)
  },
  
  update: (id, data) => {
    return http.put(`/api/users/${id}`, data)
  },
  
  delete: (id) => {
    return http.delete(`/api/users/${id}`)
  },
  
  updatePassword: (id, data) => {
    return http.put(`/api/users/${id}/password`, data)
  },
  
  login: (data) => {
    return http.post('/api/auth/login', data)
  }
}