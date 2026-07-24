/**
 * 统一通用枚举：线索（Lead）状态
 *
 * 单一事实源：所有线索生命周期状态的 label / tagType 全部从本文件读取。
 * 业务视图（whatsappBot/LeadGroupSelection、clue、marketingFlow、bulkMessaging 等）
 * 禁止再各自维护 (new/contacted/converted/lost) label map。
 *
 * 设计要点：
 * 1. value 使用业界通用的小写英文字符串（new/contacted/qualified/converted/lost/...）
 * 2. label 与 tagType 对齐 Element Plus el-tag：success/warning/danger/info/primary/''（空=默认）
 * 3. group 用于按场景过滤（funnel：完整销售漏斗；simple：极简 4 态）
 * 4. getLeadStatusLabel 找不到时回退 value，绝不返回"未知"以避免歧义
 *
 * 维护规则：新增/修改线索状态只需修改本文件即可全局生效。
 */

export const LEAD_STATUS = Object.freeze({
  NEW: 'new',               // 新线索
  CONTACTED: 'contacted',   // 已联系
  QUALIFIED: 'qualified',   // 已认证/已确认
  NEGOTIATING: 'negotiating', // 洽谈中
  CONVERTED: 'converted',   // 已转化
  LOST: 'lost',             // 已流失
  INVALID: 'invalid'        // 无效线索
})

// 完整销售漏斗（含认证/洽谈阶段），用于精细化分析
export const LEAD_STATUS_OPTIONS = Object.freeze([
  { value: 'new',         label: '新线索',   tagType: 'info',    sort: 1, group: 'funnel' },
  { value: 'contacted',   label: '已联系',   tagType: 'warning', sort: 2, group: 'funnel' },
  { value: 'qualified',   label: '已认证',   tagType: 'primary', sort: 3, group: 'funnel' },
  { value: 'negotiating', label: '洽谈中',   tagType: 'warning', sort: 4, group: 'funnel' },
  { value: 'converted',   label: '已转化',   tagType: 'success', sort: 5, group: 'funnel' },
  { value: 'lost',        label: '已流失',   tagType: 'danger',  sort: 6, group: 'funnel' },
  { value: 'invalid',     label: '无效',     tagType: 'info',    sort: 7, group: 'funnel' }
])

// 极简 4 态：兼容早期 whatsappBot/LeadGroupSelection 的硬编码列表
export const LEAD_STATUS_OPTIONS_SIMPLE = Object.freeze([
  { value: 'new',         label: '新线索',   tagType: 'info',    sort: 1, group: 'simple' },
  { value: 'contacted',   label: '已联系',   tagType: 'warning', sort: 2, group: 'simple' },
  { value: 'converted',   label: '已转化',   tagType: 'success', sort: 3, group: 'simple' },
  { value: 'lost',        label: '已流失',   tagType: 'danger',  sort: 4, group: 'simple' }
])

// value -> label 快查
export const LEAD_STATUS_LABEL_MAP = Object.freeze(
  LEAD_STATUS_OPTIONS.reduce((acc, o) => { acc[o.value] = o.label; return acc }, {})
)

// value -> tagType 快查
export const LEAD_STATUS_TAG_TYPE_MAP = Object.freeze(
  LEAD_STATUS_OPTIONS.reduce((acc, o) => { acc[o.value] = o.tagType; return acc }, {})
)

/**
 * 获取线索状态中文标签
 * @param {string|undefined|null} value 线索状态 value
 * @returns {string} label；找不到时回退 value，未知值返回 '-' 避免渲染 undefined
 */
export const getLeadStatusLabel = (value) => {
  if (value === undefined || value === null || value === '') return '-'
  return LEAD_STATUS_LABEL_MAP[value] || String(value)
}

/**
 * 获取线索状态 el-tag 类型
 * @param {string} value
 * @returns {string} tag type，找不到时返回 ''（el-tag 默认色）
 */
export const getLeadStatusTagType = (value) => LEAD_STATUS_TAG_TYPE_MAP[value] || ''

/**
 * 按 group 过滤线索状态列表
 * @param {string|string[]} groups
 * @returns {Array}
 */
export const filterLeadStatusByGroup = (groups) => {
  const list = Array.isArray(groups) ? groups : [groups]
  return LEAD_STATUS_OPTIONS.filter((o) => list.includes(o.group))
}

export default {
  LEAD_STATUS,
  LEAD_STATUS_OPTIONS,
  LEAD_STATUS_OPTIONS_SIMPLE,
  LEAD_STATUS_LABEL_MAP,
  LEAD_STATUS_TAG_TYPE_MAP,
  getLeadStatusLabel,
  getLeadStatusTagType,
  filterLeadStatusByGroup
}
