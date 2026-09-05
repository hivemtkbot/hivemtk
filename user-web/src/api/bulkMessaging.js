import { http } from '@/utils/request';

export function getTemplates(params = {}) {
  return http.get('/api/whatsapp/templates', params)
}

export function createTemplate(data) {
  return http.post('/api/whatsapp/templates', data)
}

export function updateTemplate(id, data) {
  return http.put(`/api/whatsapp/templates/${id}`, data)
}

export function deleteTemplate(id) {
  return http.delete(`/api/whatsapp/templates/${id}`)
}

export function sendBulkMessage(data) {
  return http.post('/api/whatsapp/group-messaging/send', data)
}

export function getMessageStatus(queueId) {
  return http.get(`/api/whatsapp/group-messaging/status/${queueId}`)
}

export function getSendRecords(params = {}) {
  return http.get('/api/whatsapp/group-messaging/records', params)
}