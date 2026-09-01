import { http } from '@/utils/request'

// 后端 ListRagProducts 返回 {list:[...], total:N}（项目列表规范形状），
// 历史视图层存在 Array / res.items / res.list 三种解析分支，散落 12+ 处。
// 在 API 出口单点归一化为数组，所有消费方零改动（R11 customer360 适配层先例）。
function normalizeList(res) {
  if (Array.isArray(res)) return res
  if (Array.isArray(res?.list)) return res.list
  if (Array.isArray(res?.items)) return res.items
  return res
}

// 注意：后端实际注册路径是 /api/rag-config/*（RagConfigController）
// 这里统一转发到正确路径，避免前端 404
export const ragProductConfigAPI = {
  // RAG产品管理
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
