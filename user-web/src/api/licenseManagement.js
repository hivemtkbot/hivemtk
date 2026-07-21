import { http } from '@/utils/request'

/**
 * License 授权管理 API
 * 已有 src/api/license.js 提供 GET /api/license/status，
 * 本文件补充管理类接口（授权码输入、续期申请、功能模块、商户数等）。
 */
export const LicenseManagementApi = {
  // 获取授权状态（复用已有接口路径）
  getStatus: () => {
    return http.get('/api/license/status')
  },

  // 输入/激活授权码
  activateLicense: (licenseCode) => {
    return http.post('/api/license/activate', { licenseCode })
  },

  // 续期申请
  applyRenewal: (data) => {
    return http.post('/api/license/renewal', data)
  },

  // 获取功能模块授权清单
  getModules: () => {
    return http.get('/api/license/modules')
  },

  // 获取授权商户数
  getMerchantCount: () => {
    return http.get('/api/license/merchant-count')
  },

  // 授权使用情况
  getUsage: () => {
    return http.get('/api/license/usage')
  },

  // 解绑授权
  deactivate: () => {
    return http.post('/api/license/deactivate')
  }
}
