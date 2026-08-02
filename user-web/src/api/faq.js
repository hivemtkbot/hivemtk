import { http } from '@/utils/request'

// FAQ 知识库 API - 匹配后端 /api/faqs/* 路径
// 前端 FAQ 管理页面

/**
 * @typedef {Object} FAQListParams
 * @property {number} [page]
 * @property {number} [page_size]
 * @property {string} [keyword]
 * @property {string} [category]
 * @property {string} [intent]
 * @property {boolean} [enabled]
 */

/**
 * @typedef {Object} FAQEntry
 * @property {number} id
 * @property {string} question
 * @property {string} answer
 * @property {string[]} keywords
 * @property {string} category
 * @property {string} intent
 * @property {number} confidence
 * @property {number} hit_count
 * @property {boolean} [enabled]
 * @property {string} [created_at]
 * @property {string} [updated_at]
 */

export const faqApi = {
  // 列表查询
  list(params) {
    return http.get('/api/faqs', params)
  },

  // 详情
  get(id) {
    return http.get(`/api/faqs/${id}`)
  },

  // 新增
  create(data) {
    return http.post('/api/faqs', data)
  },

  // 更新
  update(id, data) {
    return http.put(`/api/faqs/${id}`, data)
  },

  // 删除
  remove(id) {
    return http.delete(`/api/faqs/${id}`)
  },

  // Layer1 关键词匹配 (调试接口)
  match(data) {
    return http.post('/api/faqs/match', data)
  },

  // 统计
  stats() {
    return http.get('/api/faqs/stats')
  }
}
