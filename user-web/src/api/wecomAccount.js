import { http } from '@/utils/request'

// ===== 企微账号管理 & 健康度 API =====
// 字段对齐后端 wecom_health_controller.go / model.WeComAccount / model.WeComAccountHealth

/**
 * 登录状态
 * @typedef {'online' | 'offline' | 'banned'} WeComLoginState
 */

/**
 * 风险等级
 * @typedef {'normal' | 'warning' | 'critical' | 'banned'} WeComRiskLevel
 */

/**
 * 企微账号（对齐 model.WeComAccount）
 * @typedef {Object} WeComAccount
 * @property {number} id
 * @property {string} merchant_id
 * @property {string} corp_id
 * @property {string} corp_secret
 * @property {number} agent_id
 * @property {string} agent_secret
 * @property {string} access_token
 * @property {string} token_expires
 * @property {number} status
 * @property {WeComLoginState} login_state
 * @property {number} friend_count
 * @property {number} group_count
 * @property {number} daily_msg_quota
 * @property {number} daily_msg_used
 * @property {string|null} quota_reset_at
 * @property {WeComRiskLevel} risk_level
 * @property {string} risk_message
 * @property {number} weight
 * @property {string|null} last_sync_at
 * @property {string|null} last_active_at
 * @property {string|null} last_error_at
 * @property {string} last_error_msg
 * @property {number} total_sent
 * @property {number} total_received
 * @property {number} error_count
 * @property {string} created_at
 * @property {string} updated_at
 */

/**
 * 账号健康度记录（对齐 model.WeComAccountHealth）
 * @typedef {Object} WeComAccountHealth
 */

/**
 * 账号 + 最新健康度（ListAccountsWithHealth 返回项）
 * @typedef {Object} AccountWithHealth
 * @property {WeComAccount} account
 * @property {WeComAccountHealth|null} health
 */

/**
 * 健康度概览摘要（对齐 service.AccountHealthSummary）
 * @typedef {Object} AccountHealthSummary
 */

/**
 * 单账号健康概览条目
 * @typedef {Object} AccountHealthSummaryEntry
 */

/**
 * 上报健康度请求
 * @typedef {Object} ReportHealthRequest
 */

/**
 * 更新账号状态请求
 * @typedef {Object} UpdateAccountStatusRequest
 */

/**
 * 消息接入请求
 * @typedef {Object} IngestRequest
 */

/**
 * 发送消息请求
 * @typedef {Object} SendRequest
 */

/**
 * 分页响应
 * @template T
 * @typedef {Object} PageResult
 * @property {T[]} list
 * @property {number} total
 */

export const wecomAccountApi = {
  // 账号列表（含最新健康度）
  listAccounts() {
    return http.get('/api/wecom/health/accounts')
  },

  // 风险账号列表
  listRiskAccounts() {
    return http.get('/api/wecom/health/accounts/risks')
  },

  // 选择最优健康账号
  selectHealthyAccount() {
    return http.get('/api/wecom/health/accounts/select')
  },

  // 健康度概览摘要
  getHealthSummary() {
    return http.get('/api/wecom/health/accounts/summary')
  },

  // 账号最新健康度
  getLatestHealth(id) {
    return http.get(`/api/wecom/health/accounts/${id}`)
  },

  // 健康度历史（分页）
  listHealthHistory(id, params) {
    return http.get(`/api/wecom/health/accounts/${id}/history`, params)
  },

  // 上报健康度
  reportHealth(id, data) {
    return http.post(`/api/wecom/health/accounts/${id}`, data)
  },

  // 更新账号状态（login_state / risk）
  updateAccountStatus(id, data) {
    return http.post(`/api/wecom/health/accounts/${id}/status`, data)
  },

  // 消耗配额
  consumeQuota(id, count) {
    return http.post(`/api/wecom/health/accounts/${id}/quota/consume`, { count })
  },

  // 重置日配额
  resetDailyQuota() {
    return http.post('/api/wecom/health/accounts/quota/reset')
  },

  // 接入企微消息
  ingestMessage(data) {
    return http.post('/api/wecom/messages/ingest', data)
  },

  // 发送企微消息
  sendMessage(data) {
    return http.post('/api/wecom/messages/send', data)
  },

  // 创建企微账号（后端 POST /wecom/accounts -> WeComService.CreateAccount）
  // 必填 corp_id / corp_secret，可选 agent_id / agent_secret
  createAccount(data) {
    return http.post('/api/wecom/accounts', data)
  },

  // 编辑企微账号（后端 PUT /wecom/accounts/:id -> WeComService.UpdateAccount）
  updateAccount(id, data) {
    return http.put(`/api/wecom/accounts/${id}`, data)
  },

  // 删除企微账号（后端 DELETE /wecom/accounts/:id -> WeComService.DeleteAccount）
  deleteAccount(id) {
    return http.delete(`/api/wecom/accounts/${id}`)
  },

  // 企微客户列表（后端 GET /wecom/customers，分页，返回 {list,total,page,page_size}）
  getCustomers(params) {
    return http.get('/api/wecom/customers', { params })
  },

  // 企微客户群列表（后端 GET /wecom/groups，分页）
  getGroups(params) {
    return http.get('/api/wecom/groups', { params })
  },

  // 企微标签列表（后端 GET /wecom/tags，返回数组）
  getTags() {
    return http.get('/api/wecom/tags')
  },

  // 企微消息列表（后端 GET /wecom/messages，分页）
  getMessages(params) {
    return http.get('/api/wecom/messages', { params })
  },

  // 同步客户（后端 POST /wecom/accounts/:id/sync-customers）
  syncCustomers(id) {
    return http.post(`/api/wecom/accounts/${id}/sync-customers`)
  },

  // 同步客户群（后端 POST /wecom/accounts/:id/sync-groups）
  syncGroups(id) {
    return http.post(`/api/wecom/accounts/${id}/sync-groups`)
  },

  // 同步标签（后端 POST /wecom/accounts/:id/sync-tags）
  syncTags(id) {
    return http.post(`/api/wecom/accounts/${id}/sync-tags`)
  },

  // 刷新登录状态（后端 POST /wecom/accounts/:id/refresh，强制重新换取 access_token）
  refreshAccount(id) {
    return http.post(`/api/wecom/accounts/${id}/refresh`)
  },

  // 测试发送消息（后端 POST /wecom/accounts/:id/send-message）
  sendMessageById(id, data) {
    return http.post(`/api/wecom/accounts/${id}/send-message`, data)
  }
}
