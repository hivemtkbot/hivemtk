/**
 * 飞书账号管理 API
 *
 * 后端路由：/api/feishu/accounts
 * 主要能力：
 *   1. 飞书账号 CRUD（App ID / App Secret / Verification Token / Encrypt Key）
 *   2. 启用/停用 Webhook（飞书 Open Platform 事件订阅）
 *   3. 智能体开关
 *   4. 测试发送：向指定 open_id 发送消息，验证 App 凭据
 *   5. 刷新 Access Token：手动刷新 tenant_access_token
 *
 * 商业场景：商户配置飞书机器人后，飞书入站消息和事件自动触发 智能体流程
 *
 * 新增：配合 reach.feishu.send 工具，实现完整销售流程
 */

import { http } from '@/utils/request';

// 列出所有飞书账号
export function listAccounts(params = {}) {
  return http.get('/api/feishu/accounts', params)
}

// 获取账号详情
export function getAccount(id) {
  return http.get(`/api/feishu/accounts/${id}`)
}

// 创建飞书账号
export function createAccount(data) {
  return http.post('/api/feishu/accounts', data)
}

// 更新飞书账号
export function updateAccount(id, data) {
  return http.put(`/api/feishu/accounts/${id}`, data)
}

// 删除飞书账号
export function deleteAccount(id) {
  return http.delete(`/api/feishu/accounts/${id}`)
}

// 测试发送消息（验证 App 凭据）
export function testSend(id, data) {
  return http.post(`/api/feishu/accounts/${id}/test-send`, data)
}

// 刷新 Access Token
export function refreshToken(id) {
  return http.post(`/api/feishu/accounts/${id}/refresh-token`)
}
