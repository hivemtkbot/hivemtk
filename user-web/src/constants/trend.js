/**
 * 统一通用枚举：趋势（trend）
 *
 * 用于：用户增长趋势、留存趋势、指标同环比等。
 * 仅表示方向语义，与上升/下降幅度无关。
 */

export const TREND_OPTIONS = Object.freeze([
  { value: 'up',     label: '上升', tagType: 'success', description: '指标上升' },
  { value: 'down',   label: '下降', tagType: 'danger',  description: '指标下降' },
  { value: 'flat',   label: '持平', tagType: 'info',    description: '指标无明显变化' }
])

export const TREND_LABEL_MAP = Object.freeze(
  TREND_OPTIONS.reduce((acc, o) => { acc[o.value] = o.label; return acc }, {})
)

export const TREND_TAG_TYPE_MAP = Object.freeze(
  TREND_OPTIONS.reduce((acc, o) => { acc[o.value] = o.tagType; return acc }, {})
)

/**
 * 获取趋势中文标签
 * @param {string} value
 * @returns {string}
 */
export const getTrendLabel = (value) => {
  if (value === undefined || value === null || value === '') return '-'
  return TREND_LABEL_MAP[value] || String(value)
}

/**
 * 获取 el-tag 类型
 * @param {string} value
 * @returns {string}
 */
export const getTrendTagType = (value) => TREND_TAG_TYPE_MAP[value] || ''

export default {
  TREND_OPTIONS,
  TREND_LABEL_MAP,
  TREND_TAG_TYPE_MAP,
  getTrendLabel,
  getTrendTagType
}
