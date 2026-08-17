import { http } from '@/utils/request';

const BASE = '/api/whatsapp-cloud/accounts'

// 列表
export function listWhatsappCloud(params) {
  return http.get(BASE, params)
}

// 详情
export function getWhatsappCloud(id) {
  return http.get(`${BASE}/${id}`)
}

// 创建
export function createWhatsappCloud(data) {
  return http.post(BASE, data)
}

// 更新
export function updateWhatsappCloud(id, data) {
  return http.put(`${BASE}/${id}`, data)
}

// 删除
export function deleteWhatsappCloud(id) {
  return http.delete(`${BASE}/${id}`)
}

// 测试发送
export function testSendWhatsappCloud(id, data) {
  return http.post(`${BASE}/${id}/test-send`, data)
}
