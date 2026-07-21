import { http } from '@/utils/request'

/**
 * 知识库 API - 对应后端 /api/rag/*
 * 用于查询知识库文档、获取 RAG 知识库内容等
 */
export const knowledgeBaseAPI = {
  // 获取文档列表
  getDocuments(params) {
    return http.get('/api/rag/documents', { params })
  },

  // 获取单个文档
  getDocument(id) {
    return http.get(`/api/rag/documents/${id}`)
  },

  // 删除文档
  deleteDocument(id) {
    return http.delete(`/api/rag/documents/${id}`)
  },

  // 导入知识库文件
  importKnowledgeBase(data) {
    return http.post('/api/rag/import', data)
  }
}

export default knowledgeBaseAPI
