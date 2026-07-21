import { http } from '@/utils/request'

/**
 * 知识库商户管理 API - 对应后端 /api/knowledge-merchant/*
 * 商户自部署场景的可视化管理:
 *   - 批量导入(CSV/JSON)
 *   - 检索 Playground
 *   - 分段编辑
 *   - 反馈标注
 *   - API Token 管理
 *   - 外部系统接入(飞书/Notion)
 *
 * 外部接入公开入口: /api/knowledge-merchant/external/import
 *   通过 X-Knowledge-Token Header 鉴权
 */
export const knowledgeMerchantAPI = {
  // ============ 1. 批量导入 ============
  // JSON 数组结构导入
  batchImport(data) {
    return http.post('/api/knowledge-merchant/batch/import', data)
  },
  // CSV/JSON 文件上传
  batchUpload(formData) {
    return http.post('/api/knowledge-merchant/batch/upload', formData, {
      headers: { 'Content-Type': 'multipart/form-data' }
    })
  },

  // ============ 2. Playground 检索调试 ============
  playground(data) {
    return http.post('/api/knowledge-merchant/playground', data)
  },

  // ============ 3. 分段编辑 ============
  listDocumentChunks(documentId, params) {
    return http.get(`/api/knowledge-merchant/documents/${documentId}/chunks`, { params })
  },
  updateChunk(chunkId, data) {
    return http.put(`/api/knowledge-merchant/chunks/${chunkId}`, data)
  },
  deleteChunk(chunkId) {
    return http.delete(`/api/knowledge-merchant/chunks/${chunkId}`)
  },
  splitChunk(chunkId, data) {
    return http.post(`/api/knowledge-merchant/chunks/${chunkId}/split`, data)
  },

  // ============ 4. 反馈标注 ============
  submitFeedback(data) {
    return http.post('/api/knowledge-merchant/feedback', data)
  },
  listFeedbacks(params) {
    return http.get('/api/knowledge-merchant/feedbacks', { params })
  },

  // ============ 5. API Token 管理 ============
  createToken(data) {
    return http.post('/api/knowledge-merchant/tokens', data)
  },
  listTokens(params) {
    return http.get('/api/knowledge-merchant/tokens', { params })
  },
  revokeToken(tokenId) {
    return http.post(`/api/knowledge-merchant/tokens/${tokenId}/revoke`)
  },

  // ============ 6. 外部系统接入 ============
  // 鉴权由调用方在 Header 中放入 X-Knowledge-Token
  externalImport(data, token) {
    return http.post('/api/knowledge-merchant/external/import', data, {
      headers: token ? { 'X-Knowledge-Token': token } : {}
    })
  },
  listExternalJobs(params) {
    return http.get('/api/knowledge-merchant/external/jobs', { params })
  }
}

export default knowledgeMerchantAPI
