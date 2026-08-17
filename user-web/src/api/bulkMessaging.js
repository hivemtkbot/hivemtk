/**
 * 批量消息发送API
 */

import { http } from '@/utils/request';

// 获取消息模板列表
export function getTemplates(params = {}) {
  return http.get('/api/whatsapp/templates', params)
}

// 创建消息模板
export function createTemplate(data) {
  return http.post('/api/whatsapp/templates', data)
}

// 更新消息模板
export function updateTemplate(id, data) {
  return http.put(`/api/whatsapp/templates/${id}`, data)
}

// 删除消息模板
export function deleteTemplate(id) {
  return http.delete(`/api/whatsapp/templates/${id}`)
}

// 发送批量消息
export function sendBulkMessage(data) {
  return http.post('/api/whatsapp/group-messaging/send', data)
}

// 获取发送状态
export function getMessageStatus(queueId) {
  return http.get(`/api/whatsapp/group-messaging/status/${queueId}`)
}

// 获取发送记录
export function getSendRecords(params = {}) {
  return http.get('/api/whatsapp/group-messaging/records', params)
}