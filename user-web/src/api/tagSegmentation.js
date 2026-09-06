import { http } from '@/utils/request'

export const TagSegmentationApi = {
  getTags: (params) => {
    return http.get('/api/session-tags', params)
  },

  updateTags: (data) => {
    return http.put('/api/session-tags', data)
  },

  createTag: (data) => {
    return http.post('/api/session-tags', data)
  },

  deleteTag: (id) => {
    return http.delete(`/api/session-tags/${id}`)
  },

  getTagRules: (params) => {
    return http.get('/api/customer-360/tag-rules', params)
  },

  saveTagRule: (data) => {
    return http.post('/api/customer-360/tag-rules', data)
  },

  updateTagRule: (id, data) => {
    return http.put(`/api/customer-360/tag-rules/${id}`, data)
  },

  deleteTagRule: (id) => {
    return http.delete(`/api/customer-360/tag-rules/${id}`)
  },

  getLayerStrategy: () => {
    return http.get('/api/user-segment/layers')
  },

  getTagStats: (params) => {
    return http.get('/api/customer-360/tag-stats', params)
  }
};
