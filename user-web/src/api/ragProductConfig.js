import { http } from '@/utils/request'

// 注意：后端实际注册路径是 /api/rag-config/*（RagConfigController）
// 这里统一转发到正确路径，避免前端 404
export const ragProductConfigAPI = {
  // RAG产品管理
  createRagProduct(data) {
    return http.post('/api/rag-config/products', data)
  },

  getRagProducts(params) {
    return http.get('/api/rag-config/products', params)
  },

  listProducts(params) {
    return http.get('/api/rag-config/products', { params })
  },

  updateRagProduct(id, data) {
    return http.put(`/api/rag-config/products/${id}`, data)
  },

  deleteRagProduct(id) {
    return http.delete(`/api/rag-config/products/${id}`)
  },

  // 账号配置管理
  getAccountConfig(params) {
    return http.get('/api/rag-config/accounts/config', { params })
  },

  updateAccountConfig(data) {
    return http.put('/api/rag-config/accounts/config', data)
  },

  // 消息处理
  processMessage(data) {
    return http.post('/api/rag-config/process-message', data)
  }
}

export default ragProductConfigAPI
