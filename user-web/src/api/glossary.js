import { http } from '@/utils/request';

// ============================================================================
// 术语表（Glossary）管理 API
// ----------------------------------------------------------------------------
// 对应后端 /api/glossaries 路由
// 用于多语言翻译过程中的术语统一与保护（preserve=true 的术语不会被翻译）。
// ============================================================================

// 创建术语
export function createGlossary(data) {
  return http.post('/api/glossaries', data)
}

// 术语列表
// params: { category, status, keyword, page, page_size }
export function listGlossaries(params) {
  return http.get('/api/glossaries', params)
}

// 术语详情
export function getGlossary(termId) {
  return http.get(`/api/glossaries/${termId}`)
}

// 更新术语
export function updateGlossary(termId, data) {
  return http.put(`/api/glossaries/${termId}`, data)
}

// 删除术语
export function deleteGlossary(termId) {
  return http.delete(`/api/glossaries/${termId}`)
}

// 校验预览：检测文本中违规/命中术语的情况
// data: { text, lang }
export function validateGlossary(data) {
  return http.post('/api/glossaries/validate', data)
}

export default {
  createGlossary,
  listGlossaries,
  getGlossary,
  updateGlossary,
  deleteGlossary,
  validateGlossary
}
