/**
 * 统一通用枚举：客户（Customer）状态 / 标签
 *
 * 单一事实源：所有客户业务状态（active/inactive/lost/churn）以及 RFM 分层标签
 * （高价值/重要/保持/流失/潜在/...）的 label / tagType 全部从本文件读取。
 *
 * 业务视图（customer360、tagSegmentation、userSegment、whatsappBot 等）
 * 禁止再各自维护 (active/inactive/lost) label map。
 *
 * 设计要点：
 * 1. value 与后端字段保持一致（active/inactive/lost/churn/vip/high_value/keep/...）
 * 2. 兼容 enabled.js 的 0/1 二态：active/online 视为启用，inactive/disabled 视为禁用
 * 3. tagType 对齐 Element Plus el-tag：success/warning/danger/info/primary/''（空=默认）
 * 4. group 用于按场景过滤（status：客户业务状态；rfm：RFM 分层）
 *
 * 维护规则：新增/修改客户状态或标签只需修改本文件即可全局生效。
 */

export const CUSTOMER_TAG = Object.freeze({
  ACTIVE: 'active',     // 正常/活跃
  INACTIVE: 'inactive', // 不活跃
  LOST: 'lost',         // 已流失
  CHURN: 'churn',       // 已流失（与 lost 等价，部分业务字段别名）
  VIP: 'vip',           // VIP 客户
  POTENTIAL: 'potential', // 潜在客户
  HIGH_VALUE: 'high_value', // 高价值客户
  IMPORTANT: 'important',   // 重要客户
  KEEP: 'keep'          // 保持客户
})

// 客户业务状态
export const CUSTOMER_STATUS_OPTIONS = Object.freeze([
  { value: 'active',   label: '正常',     tagType: 'success', sort: 1, group: 'status' },
  { value: 'inactive', label: '不活跃',   tagType: 'info',    sort: 2, group: 'status' },
  { value: 'lost',     label: '已流失',   tagType: 'danger',  sort: 3, group: 'status' },
  { value: 'churn',    label: '已流失',   tagType: 'danger',  sort: 4, group: 'status' }
])

// RFM 分层标签
export const CUSTOMER_TAG_OPTIONS = Object.freeze([
  { value: 'vip',         label: 'VIP 客户',     tagType: 'danger',  sort: 1, group: 'rfm' },
  { value: 'high_value',  label: '高价值客户',   tagType: 'success', sort: 2, group: 'rfm' },
  { value: 'important',   label: '重要客户',     tagType: 'success', sort: 3, group: 'rfm' },
  { value: 'potential',   label: '潜在客户',     tagType: 'primary', sort: 4, group: 'rfm' },
  { value: 'keep',        label: '保持客户',     tagType: 'warning', sort: 5, group: 'rfm' },
  { value: 'lost',        label: '流失客户',     tagType: 'danger',  sort: 6, group: 'rfm' }
])

// value -> label 快查（status + tag 共用）
const _buildLabelMap = (arr) => arr.reduce((acc, o) => { acc[o.value] = o.label; return acc }, {})
const _buildTagTypeMap = (arr) => arr.reduce((acc, o) => { acc[o.value] = o.tagType; return acc }, {})

export const CUSTOMER_STATUS_LABEL_MAP = Object.freeze(_buildLabelMap(CUSTOMER_STATUS_OPTIONS))
export const CUSTOMER_STATUS_TAG_TYPE_MAP = Object.freeze(_buildTagTypeMap(CUSTOMER_STATUS_OPTIONS))
export const CUSTOMER_TAG_LABEL_MAP = Object.freeze(_buildLabelMap(CUSTOMER_TAG_OPTIONS))
export const CUSTOMER_TAG_TAG_TYPE_MAP = Object.freeze(_buildTagTypeMap(CUSTOMER_TAG_OPTIONS))

/**
 * 获取客户业务状态中文标签
 * @param {string|undefined|null} value
 * @returns {string} label；找不到时回退 value
 */
export const getCustomerStatusLabel = (value) => {
  if (value === undefined || value === null || value === '') return '-'
  return CUSTOMER_STATUS_LABEL_MAP[value] || String(value)
}

/**
 * 获取客户业务状态 el-tag 类型
 * @param {string} value
 * @returns {string}
 */
export const getCustomerStatusTagType = (value) => CUSTOMER_STATUS_TAG_TYPE_MAP[value] || ''

/**
 * 获取客户 RFM 标签中文标签
 * @param {string|undefined|null} value
 * @returns {string}
 */
export const getCustomerTagLabel = (value) => {
  if (value === undefined || value === null || value === '') return '-'
  return CUSTOMER_TAG_LABEL_MAP[value] || String(value)
}

/**
 * 获取客户 RFM 标签 el-tag 类型
 * @param {string} value
 * @returns {string}
 */
export const getCustomerTagTagType = (value) => CUSTOMER_TAG_TAG_TYPE_MAP[value] || ''

/**
 * 按 group 过滤客户状态/标签
 * @param {string|string[]} groups
 * @returns {Array}
 */
export const filterCustomerByGroup = (groups) => {
  const list = Array.isArray(groups) ? groups : [groups]
  return [...CUSTOMER_STATUS_OPTIONS, ...CUSTOMER_TAG_OPTIONS].filter((o) => list.includes(o.group))
}

export default {
  CUSTOMER_TAG,
  CUSTOMER_STATUS_OPTIONS,
  CUSTOMER_TAG_OPTIONS,
  CUSTOMER_STATUS_LABEL_MAP,
  CUSTOMER_STATUS_TAG_TYPE_MAP,
  CUSTOMER_TAG_LABEL_MAP,
  CUSTOMER_TAG_TAG_TYPE_MAP,
  getCustomerStatusLabel,
  getCustomerStatusTagType,
  getCustomerTagLabel,
  getCustomerTagTagType,
  filterCustomerByGroup
}
