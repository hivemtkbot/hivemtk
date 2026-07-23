import request from '@/utils/request'

const BASE = '/api/whatsapp-cloud/accounts'

// 列表
export function listWhatsappCloud(params) {
  return request({ url: BASE, method: 'get', params })
}

// 详情
export function getWhatsappCloud(id) {
  return request({ url: `${BASE}/${id}`, method: 'get' })
}

// 创建
export function createWhatsappCloud(data) {
  return request({ url: BASE, method: 'post', data })
}

// 更新
export function updateWhatsappCloud(id, data) {
  return request({ url: `${BASE}/${id}`, method: 'put', data })
}

// 删除
export function deleteWhatsappCloud(id) {
  return request({ url: `${BASE}/${id}`, method: 'delete' })
}

// 测试发送
export function testSendWhatsappCloud(id, data) {
  return request({ url: `${BASE}/${id}/test-send`, method: 'post', data })
}
