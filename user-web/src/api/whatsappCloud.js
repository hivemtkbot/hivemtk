import { http } from '@/utils/request';

const BASE = '/api/whatsapp-cloud/accounts'

export function listWhatsappCloud(params) {
  return http.get(BASE, params)
}

export function getWhatsappCloud(id) {
  return http.get(`${BASE}/${id}`)
}

export function createWhatsappCloud(data) {
  return http.post(BASE, data)
}

export function updateWhatsappCloud(id, data) {
  return http.put(`${BASE}/${id}`, data)
}

export function deleteWhatsappCloud(id) {
  return http.delete(`${BASE}/${id}`)
}

export function testSendWhatsappCloud(id, data) {
  return http.post(`${BASE}/${id}/test-send`, data)
}
