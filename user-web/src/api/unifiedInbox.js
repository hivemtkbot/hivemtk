import { http } from '@/utils/request'

// ===== 统一收件箱 API =====
// 注意：字段名严格对齐后端 inbox_controller.go / model.InboxConversation
// 后端查询参数为：assigned_to / pinned / starred / muted / order_by（非 is_* 前缀）

/**
 * 会话状态
 * @typedef {'unread' | 'open' | 'assigned' | 'closed'} InboxStatus
 */

/**
 * 消息来源方向
 * @typedef {'customer' | 'staff' | 'ai'} InboxMessageFrom
 */

/**
 * 分配动作
 * @typedef {'assign' | 'reassign' | 'release' | 'close' | 'reopen'} InboxAssignAction
 */

/**
 * 分配对象类型
 * @typedef {'human' | 'sop' | 'ai'} InboxAssignToType
 */

/**
 * 自动分配模式
 * @typedef {'load' | 'round_robin'} InboxAutoAssignMode
 */

/**
 * 会话列表查询参数（对齐后端 InboxQuery）
 * @typedef {Object} InboxListParams
 * @property {number} [page]
 * @property {number} [page_size]
 * @property {string} [platform]
 * @property {string} [account_id]
 * @property {string} [customer_id]
 * @property {string} [keyword]
 * @property {InboxStatus|''} [status]
 * @property {string} [assigned_to]
 * @property {number} [assigned_sop]
 * @property {string} [pinned]
 * @property {string} [starred]
 * @property {string} [muted]
 * @property {string} [order_by]
 */

/**
 * 统一收件箱会话（对齐 model.InboxConversation）
 * @typedef {Object} InboxConversation
 */

/**
 * 收件箱统计（对齐 service.InboxStats）
 * @typedef {Object} InboxStats
 */

/**
 * 分配历史记录
 * @typedef {Object} InboxAssignment
 */

/**
 * 会话消息（对齐 model.MessageHub 摘要字段）
 * @typedef {Object} InboxMessage
 */

/**
 * 客服负载响应
 * @typedef {Object} InboxStaffLoad
 * @property {string} staff
 * @property {number} load
 */

/**
 * 手动分配请求
 * @typedef {Object} InboxAssignRequest
 */

/**
 * 自动分配请求
 * @typedef {Object} InboxAutoAssignRequest
 */

/**
 * 分页列表响应
 * @template T
 * @typedef {Object} PageResult
 * @property {T[]} list
 * @property {number} total
 */

export const unifiedInboxApi = {
  // 会话列表（分页）
  listConversations(params) {
    return http.get('/api/inbox', params)
  },

  // 收件箱统计
  getStats() {
    return http.get('/api/inbox/stats')
  },

  // 分配历史列表（分页）
  listAssignments(params) {
    return http.get('/api/inbox/assignments', params)
  },

  // 手动分配（assign/reassign/release/close/reopen）
  assign(data) {
    return http.post('/api/inbox/assign', data)
  },

  // 自动分配（负载最小优先 / 轮询）
  autoAssign(data) {
    return http.post('/api/inbox/auto-assign', data)
  },

  // 客服当前负载
  getStaffLoad(staff) {
    return http.get(`/api/inbox/staff/${encodeURIComponent(staff)}/load`)
  },

  // 会话详情
  getConversation(id) {
    return http.get(`/api/inbox/${id}`)
  },

  // 标记已读
  markRead(id) {
    return http.post(`/api/inbox/${id}/read`)
  },

  // 置顶 / 取消置顶
  pin(id, pinned) {
    return http.post(`/api/inbox/${id}/pin`, { pinned })
  },

  // 星标 / 取消星标
  star(id, starred) {
    return http.post(`/api/inbox/${id}/star`, { starred })
  },

  // 免打扰 / 取消免打扰
  mute(id, muted) {
    return http.post(`/api/inbox/${id}/mute`, { muted })
  },

  // 添加标签
  addTag(id, tag) {
    return http.post(`/api/inbox/${id}/tags`, { tag })
  },

  // 移除标签
  removeTag(id, tag) {
    return http.delete(`/api/inbox/${id}/tags/${encodeURIComponent(tag)}`)
  },

  // 会话下的消息流（分页）
  listMessages(id, params) {
    return http.get(`/api/inbox/${id}/messages`, params)
  }
}
