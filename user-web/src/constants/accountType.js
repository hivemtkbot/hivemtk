/**
 * 统一通用枚举：账号（Account）状态
 *
 * 单一事实源：所有平台账号（wecom/douyin/...）的状态字段（1/2/3，active/banned/...）
 * 的 label / tagType 全部从本文件读取。
 *
 * 业务视图（platformAccount、wecomAccount、telegram、feishu、whatsapp、xiaohongshuCard 等）
 * 禁止再各自维护 (1: 正常, 2: 异常, 3: 未登录) 类小 map。
 *
 * 设计要点：
 * 1. 支持 数值型（1/2/3）和 字符串型（active/online/offline/banned/normal/warning/critical/...）
 * 2. 兼容 enabled.js：active/online/1 视为启用，inactive/disabled/0 视为禁用
 * 3. tagType 对齐 Element Plus el-tag
 * 4. group 用于按场景过滤（numeric：数字状态 1/2/3；string：英文字符串状态）
 *
 * 维护规则：新增/修改账号状态只需修改本文件即可全局生效。
 */

export const ACCOUNT_STATUS = Object.freeze({
  NORMAL: 1,           // 正常
  ABNORMAL: 2,         // 异常
  NOT_LOGGED_IN: 3,    // 未登录

  // 字符串别名（企微等使用）
  ACTIVE: 'active',
  ONLINE: 'online',
  OFFLINE: 'offline',
  BANNED: 'banned',
  WARNING: 'warning',
  CRITICAL: 'critical'
})

// 数字状态（1=正常 2=异常 3=未登录）— platformAccount/List.vue 等使用
export const ACCOUNT_STATUS_NUMERIC_OPTIONS = Object.freeze([
  { value: 1, label: '正常',   tagType: 'success', sort: 1, group: 'numeric' },
  { value: 2, label: '异常',   tagType: 'danger',  sort: 2, group: 'numeric' },
  { value: 3, label: '未登录', tagType: 'warning', sort: 3, group: 'numeric' }
])

// 字符串状态（active/online/offline/banned）— wecomAccount/List.vue 等使用
export const ACCOUNT_STATUS_STRING_OPTIONS = Object.freeze([
  { value: 'active',   label: '正常', tagType: 'success', sort: 1, group: 'string' },
  { value: 'online',   label: '在线', tagType: 'success', sort: 2, group: 'string' },
  { value: 'offline',  label: '离线', tagType: 'info',    sort: 3, group: 'string' },
  { value: 'banned',   label: '封禁', tagType: 'danger',  sort: 4, group: 'string' }
])

// 风险等级（normal/warning/critical/banned）— wecomAccount 等使用
export const ACCOUNT_RISK_LEVEL_OPTIONS = Object.freeze([
  { value: 'normal',   label: '正常', tagType: 'success', sort: 1, group: 'risk' },
  { value: 'warning',  label: '警告', tagType: 'warning', sort: 2, group: 'risk' },
  { value: 'critical', label: '危险', tagType: 'danger',  sort: 3, group: 'risk' },
  { value: 'banned',   label: '封禁', tagType: 'danger',  sort: 4, group: 'risk' }
])

// value -> label 快查（同时容纳数字与字符串）
const _buildLabelMap = (arr) => arr.reduce((acc, o) => {
  acc[o.value] = o.label
  acc[String(o.value)] = o.label
  return acc
}, {})
const _buildTagTypeMap = (arr) => arr.reduce((acc, o) => {
  acc[o.value] = o.tagType
  acc[String(o.value)] = o.tagType
  return acc
}, {})

export const ACCOUNT_STATUS_LABEL_MAP = Object.freeze({
  ..._buildLabelMap(ACCOUNT_STATUS_NUMERIC_OPTIONS),
  ..._buildLabelMap(ACCOUNT_STATUS_STRING_OPTIONS),
  ..._buildLabelMap(ACCOUNT_RISK_LEVEL_OPTIONS)
})

export const ACCOUNT_STATUS_TAG_TYPE_MAP = Object.freeze({
  ..._buildTagTypeMap(ACCOUNT_STATUS_NUMERIC_OPTIONS),
  ..._buildTagTypeMap(ACCOUNT_STATUS_STRING_OPTIONS),
  ..._buildTagTypeMap(ACCOUNT_RISK_LEVEL_OPTIONS)
})

/**
 * 获取账号状态中文 label
 * 自动识别数字（1/2/3）和字符串（active/online/offline/...），
 * 并支持企微风险等级（normal/warning/critical/banned）。
 * @param {*} value
 * @returns {string}
 */
export const getAccountStatusLabel = (value) => {
  if (value === undefined || value === null || value === '') return '-'
  return ACCOUNT_STATUS_LABEL_MAP[value] || ACCOUNT_STATUS_LABEL_MAP[String(value)] || String(value)
}

/**
 * 获取账号状态 el-tag 类型
 * @param {*} value
 * @returns {string}
 */
export const getAccountStatusTagType = (value) => {
  if (value === undefined || value === null || value === '') return ''
  return ACCOUNT_STATUS_TAG_TYPE_MAP[value] || ACCOUNT_STATUS_TAG_TYPE_MAP[String(value)] || ''
}

/**
 * 按 group 过滤账号状态列表
 * @param {string|string[]} groups
 * @returns {Array}
 */
export const filterAccountStatusByGroup = (groups) => {
  const list = Array.isArray(groups) ? groups : [groups]
  return [
    ...ACCOUNT_STATUS_NUMERIC_OPTIONS,
    ...ACCOUNT_STATUS_STRING_OPTIONS,
    ...ACCOUNT_RISK_LEVEL_OPTIONS
  ].filter((o) => list.includes(o.group))
}

export default {
  ACCOUNT_STATUS,
  ACCOUNT_STATUS_NUMERIC_OPTIONS,
  ACCOUNT_STATUS_STRING_OPTIONS,
  ACCOUNT_RISK_LEVEL_OPTIONS,
  ACCOUNT_STATUS_LABEL_MAP,
  ACCOUNT_STATUS_TAG_TYPE_MAP,
  getAccountStatusLabel,
  getAccountStatusTagType,
  filterAccountStatusByGroup
}
