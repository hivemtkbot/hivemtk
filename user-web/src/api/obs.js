import { http } from '@/utils/request';

// 获取OBS配置列表
export function getObsConfigList(params = {}) {
  return http.get('/api/obs/config', params)
}

// 获取OBS配置详情
export function getObsConfig(id) {
  return http.get(`/api/obs/config/${id}`)
}

// 创建OBS配置
export function createObsConfig(data) {
  return http.post('/api/obs/config', data)
}

// 更新OBS配置
export function updateObsConfig(id, data) {
  return http.put(`/api/obs/config/${id}`, data)
}

// 删除OBS配置
export function deleteObsConfig(id) {
  return http.delete(`/api/obs/config/${id}`)
}

// 测试OBS连接
export function testObsConnection(id) {
  return http.post(`/api/obs/config/${id}/test`)
}

// 设置默认OBS配置
export function setDefaultObsConfig(id) {
  return http.post(`/api/obs/config/${id}/default`)
}

// 获取默认OBS配置
export function getDefaultObsConfig() {
  return http.get('/api/obs/config/default')
}