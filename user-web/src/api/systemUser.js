import { http } from '@/utils/request';

export function listSystemUsers(params) {
  return http.get('/api/system/users', { params })
}

export function getSystemUser(id) {
  return http.get(`/api/system/users/${id}`)
}

export function createSystemUser(data) {
  return http.post('/api/system/users', data)
}

export function updateSystemUser(id, data) {
  return http.put(`/api/system/users/${id}`, data)
}

export function deleteSystemUser(id) {
  return http.delete(`/api/system/users/${id}`)
}

export const systemUserApi = {
  list: listSystemUsers,
  get: getSystemUser,
  create: createSystemUser,
  update: updateSystemUser,
  remove: deleteSystemUser
};

export default systemUserApi
