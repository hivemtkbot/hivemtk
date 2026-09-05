import { http } from '@/utils/request'

export const communityApi = {
  getGroups(params) {
    return http.get('/api/community/groups', { params })
  },

  createGroup(data) {
    return http.post('/api/community/groups', data)
  },

  getGroupById(id) {
    return http.get(`/api/community/groups/${id}`)
  },

  updateGroup(id, data) {
    return http.put(`/api/community/groups/${id}`, data)
  },

  deleteGroup(id) {
    return http.delete(`/api/community/groups/${id}`)
  },

  getMembers(params) {
    return http.get('/api/community/members', { params })
  },

  getMessages(params) {
    return http.get('/api/community/messages', { params })
  },

  getStats(params) {
    return http.get('/api/community/stats', { params })
  },

  importData(data) {
    return http.post('/api/community/import', data)
  },

  exportData(data) {
    return http.post('/api/community/export', data)
  }
};
