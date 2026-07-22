import { http } from '@/utils/request'

/**
 * 标签分层 API
 * 后端已有：
 *   GET /api/customer-360/tags - 获取标签
 *   PUT /api/customer-360/tags - 更新标签
 *   GET /api/user-segment - 用户分层
 */
export const TagSegmentationApi = {
  // 获取标签列表（全局标签，使用 session-tags 存储，无需 user_id）
  getTags: (params) => {
    return http.get('/api/session-tags', params)
  },

  // 更新标签（批量）
  updateTags: (data) => {
    return http.put('/api/session-tags', data)
  },

  // 新增单个标签
  createTag: (data) => {
    return http.post('/api/session-tags', data)
  },

  // 删除标签
  deleteTag: (id) => {
    return http.delete(`/api/session-tags/${id}`)
  },

  // 获取自动标签规则（条件 → 标签）
  getTagRules: (params) => {
    return http.get('/api/customer-360/tag-rules', params)
  },

  // 保存自动标签规则
  saveTagRule: (data) => {
    return http.post('/api/customer-360/tag-rules', data)
  },

  // 更新自动标签规则
  updateTagRule: (id, data) => {
    return http.put(`/api/customer-360/tag-rules/${id}`, data)
  },

  // 删除自动标签规则
  deleteTagRule: (id) => {
    return http.delete(`/api/customer-360/tag-rules/${id}`)
  },

  // 获取分层策略（RFM 分层 → 标签组）
  getLayerStrategy: () => {
    return http.get('/api/user-segment/layers')
  },

  // 保存分层策略
  saveLayerStrategy: (data) => {
    return http.put('/api/user-segment/layers', data)
  },

  // 标签统计
  getTagStats: (params) => {
    return http.get('/api/customer-360/tag-stats', params)
  }
}
