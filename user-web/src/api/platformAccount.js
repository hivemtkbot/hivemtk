import { http } from '@/utils/request'

// 平台账号管理 API 接口
export const platformAccountApi = {
  // 获取平台账号列表
  getAccounts(params) {
    return http.get('/api/platform-accounts', { params })
  },

  // 获取支持的平台列表
  getPlatforms() {
    return http.get('/api/platform-accounts/platforms')
  },

  // 获取平台账号详情
  getAccountById(id) {
    return http.get(`/api/platform-accounts/${id}`)
  },

  // 创建平台账号
  createAccount(data) {
    return http.post('/api/platform-accounts', data)
  },

  // 更新平台账号
  updateAccount(id, data) {
    return http.put(`/api/platform-accounts/${id}`, data)
  },

  // 删除平台账号
  deleteAccount(id) {
    return http.delete(`/api/platform-accounts/${id}`)
  },

  // 登录平台账号
  loginAccount(id, data) {
    return http.post(`/api/platform-accounts/${id}/login`, data)
  },

  // 检查平台账号状态
  checkStatus(id) {
    return http.get(`/api/platform-accounts/${id}/status`)
  }
}
