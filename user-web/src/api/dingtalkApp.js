import { http } from '@/utils/request';

const BASE = '/api/dingtalk-app/accounts'

export function listDingtalkApp(params) {
  return http.get(BASE, params)
}

export function getDingtalkApp(id) {
  return http.get(`${BASE}/${id}`)
}

export function createDingtalkApp(data) {
  return http.post(BASE, data)
}

export function updateDingtalkApp(id, data) {
  return http.put(`${BASE}/${id}`, data)
}

export function deleteDingtalkApp(id) {
  return http.delete(`${BASE}/${id}`)
}

export function testDingtalkApp(id) {
  return http.post(`${BASE}/${id}/test`)
}
