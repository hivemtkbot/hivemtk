import http from '@/utils/request'

// 触发系统重置（清空本地配置，跳转初始化）
export function resetSystem() {
  return http.post('/api/system/reset')
}

// 站点 / SEO / 客服基础配置
export const SystemApi = {
  getConfig() {
    return http.get('/api/system/config')
  },
  saveConfig(data) {
    return http.post('/api/system/config', data)
  },
}
