import { http } from '@/utils/request'

/**
 * OTA 升级 API
 * 已有 src/api/platformVersion.js 提供版本相关接口，
 * 本文件聚焦 OTA 升级流程（检查更新、升级操作、回滚等）。
 */
export const OtaUpgradeApi = {
  // 当前版本信息
  getCurrentVersion: () => {
    return http.get('/api/platform/version/latest')
  },

  // 版本历史列表
  getVersionHistory: (params) => {
    return http.get('/api/platform/version/list', params)
  },

  // 版本详情
  getVersionDetail: (id) => {
    return http.get(`/api/platform/version/${id}`)
  },

  // 检查更新
  checkUpdate: (params) => {
    return http.get('/api/platform/version/check-update', params)
  },

  // 执行升级
  doUpgrade: (data) => {
    return http.post('/api/upgrade', data)
  },

  // 升级进度查询
  getUpgradeProgress: (taskId) => {
    return http.get(`/api/upgrade/progress/${taskId}`)
  },

  // 回滚到指定版本
  rollback: (version) => {
    return http.post('/api/upgrade/rollback', { version })
  },

  // 升级历史记录
  getUpgradeHistory: (params) => {
    return http.get('/api/upgrade/history', params)
  }
}
