/**
 * Telegram Bot 账号管理 API
 *
 * 后端路由：/api/telegram/accounts
 * 主要能力：
 *   1. Bot 账号 CRUD（Bot Token / Webhook URL / Webhook Secret）
 *   2. Webhook 注册：调用 Telegram setWebhook 接口
 *   3. 测试发送：向指定 chat_id 发送消息，验证 Bot Token
 *
 * 商业场景：商户配置 TG Bot 后，TG 入站消息和入群事件会自动触发 智能体流程
 */

import { http } from '@/utils/request';

// 列出所有 Bot 账号
export function listAccounts(params = {}) {
  return request({
    url: '/api/telegram/accounts',
    method: 'get',
    params
  })
}

// 获取账号详情
export function getAccount(id) {
  return http.get(`/api/telegram/accounts/${id}`)
}

// 创建 Bot 账号
export function createAccount(data) {
  return request({
    url: '/api/telegram/accounts',
    method: 'post',
    data
  })
}

// 更新 Bot 账号
export function updateAccount(id, data) {
  return request({
    url: `/api/telegram/accounts/${id}`,
    method: 'put',
    data
  })
}

// 删除 Bot 账号
export function deleteAccount(id) {
  return http.delete(`/api/telegram/accounts/${id}`)
}

// 注册 Webhook（调用 Telegram setWebhook）
export function registerWebhook(id, data = {}) {
  return request({
    url: `/api/telegram/accounts/${id}/register-webhook`,
    method: 'post',
    data
  })
}

// 测试发送消息
export function testSend(id, data) {
  return request({
    url: `/api/telegram/accounts/${id}/test-send`,
    method: 'post',
    data
  })
}
