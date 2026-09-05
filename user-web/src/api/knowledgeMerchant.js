import { http } from '@/utils/request'

export const knowledgeMerchantAPI = {
  batchImport(data) {
    return http.post('/api/knowledge-merchant/batch/import', data)
  },
  batchUpload(formData) {
    return http.post('/api/knowledge-merchant/batch/upload', formData, {
      headers: { 'Content-Type': 'multipart/form-data' }
    })
  },

  playground(data) {
    return http.post('/api/knowledge-merchant/playground', data)
  },

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

  submitFeedback(data) {
    return http.post('/api/knowledge-merchant/feedback', data)
  },
  listFeedbacks(params) {
    return http.get('/api/knowledge-merchant/feedbacks', { params })
  },

  createToken(data) {
    return http.post('/api/knowledge-merchant/tokens', data)
  },
  listTokens(params) {
    return http.get('/api/knowledge-merchant/tokens', { params })
  },
  revokeToken(tokenId) {
    return http.post(`/api/knowledge-merchant/tokens/${tokenId}/revoke`)
  },

  externalImport(data, token) {
    return http.post('/api/knowledge-merchant/external/import', data, {
      headers: token ? { 'X-Knowledge-Token': token } : {}
    })
  },
  listExternalJobs(params) {
    return http.get('/api/knowledge-merchant/external/jobs', { params })
  }
};

export default knowledgeMerchantAPI
