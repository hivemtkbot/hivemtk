import { http } from '@/utils/request'

// SOP 模板管理 API - 匹配后端 /api/sop-templates/* 路径
// 前端 SOP 模板管理页面

/**
 * @typedef {Object} SOPTemplateListParams
 * @property {number} [page]
 * @property {number} [page_size]
 * @property {string} [keyword]
 * @property {string} [intent]
 * @property {string} [stage]
 * @property {boolean} [enabled]
 */

/**
 * @typedef {Object} SOPTemplate
 * @property {number} id
 * @property {string} name
 * @property {string} intent
 * @property {string} stage
 * @property {string} template
 * @property {string} vars
 * @property {number} priority
 * @property {number} confidence
 * @property {number} hit_count
 * @property {boolean} [enabled]
 * @property {string} [created_at]
 * @property {string} [updated_at]
 */

export const sopTemplateApi = {
  // 列表查询
  list(params) {
    return http.get('/api/sop-templates', params)
  },

  // 详情
  get(id) {
    return http.get(`/api/sop-templates/${id}`)
  },

  // 新增
  create(data) {
    return http.post('/api/sop-templates', data)
  },

  // 更新
  update(id, data) {
    return http.put(`/api/sop-templates/${id}`, data)
  },

  // 删除
  remove(id) {
    return http.delete(`/api/sop-templates/${id}`)
  },

  // Layer1 (intent, stage) 匹配 (调试接口)
  match(data) {
    return http.post('/api/sop-templates/match', data)
  }
}
