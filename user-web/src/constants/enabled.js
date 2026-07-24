/**
 * 统一通用枚举：启用/禁用
 *
 * 大多数业务实体的 status 字段是 0/1 或 'active'/'disabled' 形式的二态开关。
 * 业务视图禁止再各自维护 `({ 1: '启用', 0: '禁用' })` 类小 map。
 *
 * 兼容性：本模块同时支持 数值型（1/0）和 字符串型（active/disabled/inactive/online/offline），
 * 调用方只需 import 一次。
 *
 * 用法：
 *   import { getEnabledLabel, getEnabledTagType, isEnabled } from '@/constants/enabled'
 *   {{ getEnabledLabel(row.status) }}
 *   :type="getEnabledTagType(row.status)"
 */

import i18n from '@/i18n'

export const ENABLED_OPTIONS = Object.freeze([
  { value: 1,        label: '启用',   tagType: 'success', aliases: ['active', 'enabled', 'online', 'on', 1, '1', true] },
  { value: 0,        label: '禁用',   tagType: 'danger',  aliases: ['inactive', 'disabled', 'offline', 'off', 0, '0', false] }
])

// alias -> 标准 value 的反向索引
const ALIAS_TO_VALUE = Object.freeze(
  ENABLED_OPTIONS.reduce((acc, o) => {
    o.aliases.forEach((a) => { acc[String(a).toLowerCase()] = o.value })
    return acc
  }, {})
)

// value/alias -> label
export const ENABLED_LABEL_MAP = Object.freeze(
  ENABLED_OPTIONS.reduce((acc, o) => {
    acc[String(o.value)] = o.label
    o.aliases.forEach((a) => { acc[String(a).toLowerCase()] = o.label })
    return acc
  }, {})
)

// value/alias -> tagType
export const ENABLED_TAG_TYPE_MAP = Object.freeze(
  ENABLED_OPTIONS.reduce((acc, o) => {
    acc[String(o.value)] = o.tagType
    o.aliases.forEach((a) => { acc[String(a).toLowerCase()] = o.tagType })
    return acc
  }, {})
)

/**
 * 标准化 value（识别 1/0/'active'/'disabled'/'on'/'off'/true/false 等）
 * @param {*} v
 * @returns {number} 0 或 1，无法识别返回 NaN
 */
export const normalizeEnabled = (v) => {
  if (typeof v === 'number') return v === 1 ? 1 : 0
  if (typeof v === 'boolean') return v ? 1 : 0
  const key = String(v ?? '').toLowerCase()
  if (key in ALIAS_TO_VALUE) return ALIAS_TO_VALUE[key]
  return NaN
}

/**
 * @param {*} v
 * @returns {boolean}
 */
export const isEnabled = (v) => normalizeEnabled(v) === 1

/**
 * 获取启用/禁用中文 label
 * @param {*} v
 * @returns {string}
 */
export const getEnabledLabel = (v) => {
  if (v === undefined || v === null || v === '') return '-'
  const n = normalizeEnabled(v)
  if (n === 1 || n === 0) {
    const key = n === 1 ? 'enabledLabel.enabled' : 'enabledLabel.disabled'
    const tr = i18n.global.t(key)
    if (tr && tr !== key) return tr
  }
  const key = String(v).toLowerCase()
  return ENABLED_LABEL_MAP[key] || String(v)
}

/**
 * 获取 el-tag 类型
 * @param {*} v
 * @returns {string}
 */
export const getEnabledTagType = (v) => {
  const key = String(v ?? '').toLowerCase()
  return ENABLED_TAG_TYPE_MAP[key] || ''
}

export default {
  ENABLED_OPTIONS,
  ENABLED_LABEL_MAP,
  ENABLED_TAG_TYPE_MAP,
  getEnabledLabel,
  getEnabledTagType,
  isEnabled,
  normalizeEnabled
}
