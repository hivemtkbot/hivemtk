import request from '@/utils/request'

// ============================================================================
// 术语表（Glossary）管理 API
// ----------------------------------------------------------------------------
// 对应后端 /api/glossaries 路由
// 用于多语言翻译过程中的术语统一与保护（preserve=true 的术语不会被翻译）。
// ============================================================================

// 创建术语
export function createGlossary(data) {
  return request({
    url: '/api/glossaries',
    method: 'post',
    data
  })
}

// 术语列表
// params: { category, status, keyword, page, page_size }
export function listGlossaries(params) {
  return request({
    url: '/api/glossaries',
    method: 'get',
    params
  })
}

// 术语详情
export function getGlossary(termId) {
  return request({
    url: `/api/glossaries/${termId}`,
    method: 'get'
  })
}

// 更新术语
export function updateGlossary(termId, data) {
  return request({
    url: `/api/glossaries/${termId}`,
    method: 'put',
    data
  })
}

// 删除术语
export function deleteGlossary(termId) {
  return request({
    url: `/api/glossaries/${termId}`,
    method: 'delete'
  })
}

// 校验预览：检测文本中违规/命中术语的情况
// data: { text, lang }
export function validateGlossary(data) {
  return request({
    url: '/api/glossaries/validate',
    method: 'post',
    data
  })
}

export default {
  createGlossary,
  listGlossaries,
  getGlossary,
  updateGlossary,
  deleteGlossary,
  validateGlossary
}
