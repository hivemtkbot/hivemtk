import { http } from '@/utils/request'

/**
 * 知识库 API - 对应后端 /api/knowledge/*
 * 涵盖:文档导入(上传/文本/URL)、文档管理、检索、OpenAPI 数据源、统计
 */
export const knowledgeAPI = {
  // ============ 文档导入 ============
  // 上传文件导入
  importUpload(formData) {
    return http.post('/api/knowledge/import/upload', formData, {
      headers: { 'Content-Type': 'multipart/form-data' }
    })
  },
  // 文本导入
  importText(data) {
    return http.post('/api/knowledge/import/text', data)
  },
  // URL 导入
  importURL(data) {
    return http.post('/api/knowledge/import/url', data)
  },

  // ============ 文档管理 ============
  listDocuments(params) {
    return http.get('/api/knowledge/documents', { params })
  },
  getDocument(id, params) {
    return http.get(`/api/knowledge/documents/${id}`, { params })
  },
  getDocumentProgress(id) {
    return http.get(`/api/knowledge/documents/${id}/progress`)
  },
  getDocumentChunks(id) {
    return http.get(`/api/knowledge/documents/${id}/chunks`)
  },
  updateDocument(id, data) {
    return http.put(`/api/knowledge/documents/${id}`, data)
  },
  deleteDocument(id, params) {
    return http.delete(`/api/knowledge/documents/${id}`, { params })
  },
  reindexDocument(id, params) {
    return http.post(`/api/knowledge/documents/${id}/reindex`, null, { params })
  },

  // ============ 产品级 ============
  rebuildProductIndex(productId) {
    return http.post(`/api/knowledge/products/${productId}/rebuild-index`)
  },
  getProductOverview(productId) {
    return http.get(`/api/knowledge/products/${productId}/overview`)
  },

  // ============ 检索 ============
  search(data) {
    return http.post('/api/knowledge/search', data)
  },

  // ============ 导入日志 ============
  listImportLogs(params) {
    return http.get('/api/knowledge/import-logs', { params })
  },

  // ============ OpenAPI 数据源 ============
  listOpenAPISources(params) {
    return http.get('/api/knowledge/openapi/sources', { params })
  },
  createOpenAPISource(data) {
    return http.post('/api/knowledge/openapi/sources', data)
  },
  getOpenAPISource(id, params) {
    return http.get(`/api/knowledge/openapi/sources/${id}`, { params })
  },
  updateOpenAPISource(id, data) {
    return http.put(`/api/knowledge/openapi/sources/${id}`, data)
  },
  deleteOpenAPISource(id, params) {
    return http.delete(`/api/knowledge/openapi/sources/${id}`, { params })
  },
  syncOpenAPISource(id, params) {
    return http.post(`/api/knowledge/openapi/sources/${id}/sync`, null, { params })
  },
  testOpenAPISource(data) {
    return http.post('/api/knowledge/openapi/sources/test', data)
  },
  toggleOpenAPISource(id, data, params) {
    return http.post(`/api/knowledge/openapi/sources/${id}/toggle`, data, { params })
  },

  // ============ 统计 ============
  getOverviewStats(params) {
    return http.get('/api/knowledge/stats/overview', { params })
  },
  getDocumentStats(params) {
    return http.get('/api/knowledge/stats/documents', { params })
  },
  getSearchStats(params) {
    return http.get('/api/knowledge/stats/searches', { params })
  },
  getImportStats(params) {
    return http.get('/api/knowledge/stats/imports', { params })
  },
  getOpenAPIStats(params) {
    return http.get('/api/knowledge/stats/openapi', { params })
  }
}

export default knowledgeAPI
