import { http } from '@/utils/request'

// ============================================================================
// 知识库 API（多类型统一管理）
// ----------------------------------------------------------------------------
// 知识库类型（kb_type）：
//   - rag    ：RAG 文档库（关联 rag_products / knowledge 文档）
//   - faq    ：FAQ 知识库（关联 faqs）
//   - sop    ：SOP 模板库（关联 sop_templates）
// 后端路由前缀：/api/knowledge-bases/*
// ============================================================================

// 列表查询（支持分页 / 类型筛选 / 关键字搜索）
export function listKBs(params) {
  return http.get('/api/knowledge-bases', params)
}

// 按类型查询（便于编辑页 / 智能体绑定时按 RAG/FAQ/SOP 分组）
export function listByType(kbType, params = {}) {
  return http.get(`/api/knowledge-bases/by-type/${kbType}`, params)
}

// 知识库详情
export function getKB(id) {
  return http.get(`/api/knowledge-bases/${id}`)
}

// 创建知识库
export function createKB(data) {
  return http.post('/api/knowledge-bases', data)
}

// 更新知识库
export function updateKB(id, data) {
  return http.put(`/api/knowledge-bases/${id}`, data)
}

// 删除知识库
export function deleteKB(id) {
  return http.delete(`/api/knowledge-bases/${id}`)
}

// 知识库总览统计（文档/FAQ/SOP 数 + 关联智能体数）
export function getKBStats(id) {
  return http.get(`/api/knowledge-bases/${id}/stats`)
}

export const knowledgeBaseAPI = {
  listKBs,
  getKB,
  createKB,
  updateKB,
  deleteKB,
  listByType,
  getKBStats
}

export default knowledgeBaseAPI
