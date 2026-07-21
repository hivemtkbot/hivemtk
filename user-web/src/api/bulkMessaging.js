/**
 * 批量消息发送API
 */

import request from '@/utils/request'

// 获取消息模板列表
export function getTemplates(params = {}) {
  return request({
    url: '/api/whatsapp/templates',
    method: 'get',
    params
  })
}

// 创建消息模板
export function createTemplate(data) {
  return request({
    url: '/api/whatsapp/templates',
    method: 'post',
    data
  })
}

// 更新消息模板
export function updateTemplate(id, data) {
  return request({
    url: `/api/whatsapp/templates/${id}`,
    method: 'put',
    data
  })
}

// 删除消息模板
export function deleteTemplate(id) {
  return request({
    url: `/api/whatsapp/templates/${id}`,
    method: 'delete'
  })
}

// 发送批量消息
export function sendBulkMessage(data) {
  return request({
    url: '/api/whatsapp/group-messaging/send',
    method: 'post',
    data
  })
}

// 获取发送状态
export function getMessageStatus(queueId) {
  return request({
    url: `/api/whatsapp/group-messaging/status/${queueId}`,
    method: 'get'
  })
}

// 获取发送记录
export function getSendRecords(params = {}) {
  return request({
    url: '/api/whatsapp/group-messaging/records',
    method: 'get',
    params
  })
}