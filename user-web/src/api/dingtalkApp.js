import { http } from '@/utils/request';

const BASE = '/api/dingtalk-app/accounts'

// 列表
export function listDingtalkApp(params) {
  return http.get(BASE, params)
}

// 详情
export function getDingtalkApp(id) {
  return http.get(`${BASE}/${id}`)
}

// 创建
export function createDingtalkApp(data) {
  return http.post(BASE, data)
}

// 更新
export function updateDingtalkApp(id, data) {
  return http.put(`${BASE}/${id}`, data)
}

// 删除
export function deleteDingtalkApp(id) {
  return http.delete(`${BASE}/${id}`)
}

// 测试配置（校验必填项）
export function testDingtalkApp(id) {
  return http.post(`${BASE}/${id}/test`)
}
