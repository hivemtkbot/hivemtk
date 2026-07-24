/**
 * 统一通用枚举：订单（Order）状态
 *
 * 单一事实源：所有订单/任务/作业（短信/WhatsApp/邮件）状态字段
 * （pending/running/paused/completed/failed/cancelled/...）的 label / tagType
 * 全部从本文件读取。
 *
 * 业务视图（sms/Jobs、sms/List、whatsapp/WhatsappJobs、batchOperation、marketingFlow 等）
 * 禁止再各自维护 ({ pending: '待执行', ... }) 类小 map。
 *
 * 设计要点：
 * 1. value 与后端字段保持一致（小写英文字符串）
 * 2. 完整覆盖：pending/sending/running/paused/completed/failed/cancelled/scheduled
 * 3. tagType 对齐 Element Plus el-tag
 * 4. group 用于按场景过滤（sms：短信作业；job：通用作业）
 *
 * 与 status.js 的 TASK_STATUS 协同：本模块提供更细分的"作业/订单"状态集合，
 * TASK_STATUS 是更通用抽象；本模块侧重"业务级订单状态"业务可读性。
 *
 * 维护规则：新增/修改订单状态只需修改本文件即可全局生效。
 */

export const ORDER_STATUS = Object.freeze({
  PENDING: 'pending',       // 待执行 / 待发送
  SCHEDULED: 'scheduled',   // 已计划（定时）
  SENDING: 'sending',       // 发送中（仅限消息作业）
  SENT: 'sent',             // 已发送（短信/消息作业）
  RUNNING: 'running',       // 进行中
  PAUSED: 'paused',         // 已暂停
  COMPLETED: 'completed',   // 已完成
  FAILED: 'failed',         // 已失败
  CANCELLED: 'cancelled'    // 已取消
})

// 通用作业/订单状态（覆盖 SMS Jobs、Whatsapp Jobs、邮件 Jobs 等）
export const ORDER_STATUS_OPTIONS = Object.freeze([
  { value: 'pending',    label: '待执行',   tagType: 'info',    sort: 1, group: 'job' },
  { value: 'scheduled',  label: '已计划',   tagType: 'info',    sort: 2, group: 'job' },
  { value: 'sending',    label: '发送中',   tagType: 'warning', sort: 3, group: 'sms' },
  { value: 'sent',       label: '已发送',   tagType: 'success', sort: 4, group: 'sms' },
  { value: 'running',    label: '执行中',   tagType: 'warning', sort: 5, group: 'job' },
  { value: 'paused',     label: '已暂停',   tagType: 'info',    sort: 6, group: 'job' },
  { value: 'completed',  label: '已完成',   tagType: 'success', sort: 7, group: 'job' },
  { value: 'failed',     label: '已失败',   tagType: 'danger',  sort: 8, group: 'job' },
  { value: 'cancelled',  label: '已取消',   tagType: 'info',    sort: 9, group: 'job' }
])

// value -> label 快查
export const ORDER_STATUS_LABEL_MAP = Object.freeze(
  ORDER_STATUS_OPTIONS.reduce((acc, o) => { acc[o.value] = o.label; return acc }, {})
)

// value -> tagType 快查
export const ORDER_STATUS_TAG_TYPE_MAP = Object.freeze(
  ORDER_STATUS_OPTIONS.reduce((acc, o) => { acc[o.value] = o.tagType; return acc }, {})
)

/**
 * 获取订单/作业状态中文 label
 * @param {string|undefined|null} value
 * @returns {string} label；找不到时回退 value
 */
export const getOrderStatusLabel = (value) => {
  if (value === undefined || value === null || value === '') return '-'
  return ORDER_STATUS_LABEL_MAP[value] || String(value)
}

/**
 * 获取订单/作业状态 el-tag 类型
 * @param {string} value
 * @returns {string}
 */
export const getOrderStatusTagType = (value) => ORDER_STATUS_TAG_TYPE_MAP[value] || ''

/**
 * 按 group 过滤订单状态列表
 * @param {string|string[]} groups
 * @returns {Array}
 */
export const filterOrderStatusByGroup = (groups) => {
  const list = Array.isArray(groups) ? groups : [groups]
  return ORDER_STATUS_OPTIONS.filter((o) => list.includes(o.group))
}

export default {
  ORDER_STATUS,
  ORDER_STATUS_OPTIONS,
  ORDER_STATUS_LABEL_MAP,
  ORDER_STATUS_TAG_TYPE_MAP,
  getOrderStatusLabel,
  getOrderStatusTagType,
  filterOrderStatusByGroup
}

