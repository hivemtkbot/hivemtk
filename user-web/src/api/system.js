import http from '@/utils/request'

// 站点 / SEO / 客服基础配置
export const SystemApi = {
  getConfig() {
    return http.get('/api/system/config')
  },
  saveConfig(data) {
    return http.post('/api/system/config', data)
  },
}
