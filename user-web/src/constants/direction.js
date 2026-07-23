/**
 * 统一通用枚举：方向（direction）
 *
 * 业务：messageHub 中台消息方向（in/out）、unifiedInbox 消息方向
 */

export const DIRECTION_OPTIONS = Object.freeze([
  { value: 'in',       label: '入站',     tagType: 'success', description: '客户发来' },
  { value: 'inbound',  label: '入站',     tagType: 'success', description: '客户发来' },
  { value: 'out',      label: '出站',     tagType: 'info',    description: '客服发出' },
  { value: 'outbound', label: '出站',     tagType: 'info',    description: '客服发出' }
])

export const DIRECTION_LABEL_MAP = Object.freeze(
  DIRECTION_OPTIONS.reduce((acc, o) => { acc[o.value] = o.label; return acc }, {})
)

export const DIRECTION_TAG_TYPE_MAP = Object.freeze(
  DIRECTION_OPTIONS.reduce((acc, o) => { acc[o.value] = o.tagType; return acc }, {})
)

export const getDirectionLabel = (v) => {
  if (v === undefined || v === null || v === '') return '-'
  return DIRECTION_LABEL_MAP[v] || String(v)
}

export const getDirectionTagType = (v) => DIRECTION_TAG_TYPE_MAP[v] || ''

export default {
  DIRECTION_OPTIONS,
  DIRECTION_LABEL_MAP,
  DIRECTION_TAG_TYPE_MAP,
  getDirectionLabel,
  getDirectionTagType
}
