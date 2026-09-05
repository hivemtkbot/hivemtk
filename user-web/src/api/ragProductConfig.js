import { http } from '@/utils/request'

function normalizeList(res) {
  if (Array.isArray(res)) return res
  if (Array.isArray(res?.list)) return res.list
  if (Array.isArray(res?.items)) return res.items
  return res
}

export const ragProductConfigAPI = {
  createRagProduct(data) {
    return http.post('/api/rag-config/products', data)
  },

  getRagProducts(params) {
    return http.get('/api/rag-config/products', params).then(normalizeList)
  },

  listProducts(params) {
    return http.get('/api/rag-config/products', { params }).then(normalizeList)
  },

  updateRagProduct(id, data) {
    return http.put(`/api/rag-config/products/${id}`, data)
  },

  deleteRagProduct(id) {
    return http.delete(`/api/rag-config/products/${id}`)
  },

  getAccountConfig(params) {
    return http.get('/api/rag-config/accounts/config', { params })
  },

  updateAccountConfig(data) {
    return http.put('/api/rag-config/accounts/config', data)
  },

  processMessage(data) {
    return http.post('/api/rag-config/process-message', data)
  }
};

export default ragProductConfigAPI
