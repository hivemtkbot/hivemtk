/**
 * 统一系统级"来源（Source）"常量
 *
 * 单一事实源：所有系统级来源（auto/manual/system/llm/rule/api/crawler/upload/imported/synced/...）
 * 的 label 与 tagType 全部从本文件读取。
 *
 * 注意：本模块不处理"渠道/平台"维度的来源（douyin/xiaohongshu/wecom 等），
 * 那种语义请使用 src/constants/channel.js 的 getChannelLabel。
 *
 * 适用范围：
 * - 客户事件来源（auto/manual/system）
 * - 客户旅程事件来源（auto/manual）
 * - 黑名单来源（auto/manual）
 * - AI 建议来源（llm/rule）
 * - 知识库来源（upload/text/url/openapi/batch/api/crawler）
 * - 批量操作数据来源（import/api/manual）
 * - 资产市场来源（purchased/manual/synced/imported）
 *
 * 维护规则：新增来源时只需在本文件中追加即可全局生效。
 */

export const SOURCE_OPTIONS = Object.freeze([
  // === 通用来源 ===
  { value: 'auto',      label: '系统自动',   tagType: 'warning', group: 'common', description: '系统自动生成/触发' },
  { value: 'manual',    label: '手动',       tagType: 'info',    group: 'common', description: '人工操作产生' },
  { value: 'system',    label: '系统',       tagType: 'info',    group: 'common', description: '系统内部事件' },
  { value: 'rule',      label: '规则',       tagType: '',        group: 'common', description: '规则引擎生成' },
  { value: 'llm',       label: 'AI',         tagType: 'primary', group: 'common', description: 'AI/LLM 生成' },

  // === 知识库来源 ===
  { value: 'upload',    label: '文件上传',   tagType: 'info',    group: 'knowledge', description: '本地文件上传' },
  { value: 'text',      label: '文本输入',   tagType: 'success', group: 'knowledge', description: '在线文本输入' },
  { value: 'url',       label: 'URL 抓取',   tagType: 'warning', group: 'knowledge', description: 'URL 网页抓取' },
  { value: 'openapi',   label: 'OpenAPI',    tagType: 'info',    group: 'knowledge', description: '外部 OpenAPI 同步' },
  { value: 'batch',     label: '批量导入',   tagType: '',        group: 'knowledge', description: '批量任务导入' },
  { value: 'api',       label: 'API',        tagType: 'info',    group: 'knowledge', description: '外部 API 推送' },
  { value: 'crawler',   label: '爬虫',       tagType: 'warning', group: 'knowledge', description: '爬虫采集' },

  // === 批量操作数据来源 ===
  { value: 'import',    label: '导入',       tagType: 'info',    group: 'batch', description: '批量导入' },
  { value: 'webhook',   label: 'Webhook',    tagType: 'success', group: 'batch', description: 'Webhook 回调' },

  // === 资产市场来源 ===
  { value: 'purchased', label: '平台购买',   tagType: 'success', group: 'asset', description: '从平台购买' },
  { value: 'synced',    label: '平台分发',   tagType: 'success', group: 'asset', description: '平台同步分发' },
  { value: 'imported',  label: '导入',       tagType: 'info',    group: 'asset', description: '本地导入' }
])

// value -> label 快查
export const SOURCE_LABEL_MAP = Object.freeze(
  SOURCE_OPTIONS.reduce((acc, o) => { acc[o.value] = o.label; return acc }, {})
)

// value -> tagType 快查
export const SOURCE_TAG_TYPE_MAP = Object.freeze(
  SOURCE_OPTIONS.reduce((acc, o) => { acc[o.value] = o.tagType; return acc }, {})
)

/**
 * 获取来源中文标签
 * @param {string|undefined|null} value 来源 value
 * @returns {string} label；找不到时回退 value（不显示"未知"以免误导）
 */
export const getSourceLabel = (value) => {
  if (value === undefined || value === null || value === '') return '-'
  return SOURCE_LABEL_MAP[value] || value
}

/**
 * 获取来源 el-tag 类型
 * @param {string} value
 * @returns {string}
 */
export const getSourceTagType = (value) => SOURCE_TAG_TYPE_MAP[value] || ''

/**
 * 按 group 过滤来源选项
 * @param {string|string[]} groups
 * @returns {Array}
 */
export const filterSourcesByGroup = (groups) => {
  const list = Array.isArray(groups) ? groups : [groups]
  return SOURCE_OPTIONS.filter((o) => list.includes(o.group))
}

export default {
  SOURCE_OPTIONS,
  SOURCE_LABEL_MAP,
  SOURCE_TAG_TYPE_MAP,
  getSourceLabel,
  getSourceTagType,
  filterSourcesByGroup
}
