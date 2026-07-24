/**
 * 统一通用枚举：卡片平台（Card Platform）
 *
 * 单一事实源：所有"卡片/线索/账号"业务平台（douyin/kuaishou/xiaohongshu/xianyu/tiktok）
 * 的 label / tagType 全部从本文件读取。
 *
 * 业务视图（douyinCard、kuaishouCard、xiaohongshuCard、xianyuCard、tiktokCard、
 * RagProductConfig/AccountConfig、clue、unifiedMessage 等）禁止再各自维护
 * inline `<el-option label="抖音" value="douyin" />` 列表。
 *
 * 与 channel.js 的关系：
 * - channel.js 负责"消息触达渠道"（wecom/whatsapp/sms/...），侧重 IM 通道
 * - cardPlatform.js 负责"卡片/线索/账号业务平台"（douyin/kuaishou/...），侧重内容平台
 * - 两者在 douyin/kuaishou/xiaohongshu/xianyu/tiktok 这 5 个值上重合，label/tagType 保持一致
 *
 * 设计要点：
 * 1. value 使用业界通用小写英文（douyin/kuaishou/xiaohongshu/xianyu/tiktok）
 * 2. label 与前端展示一致（抖音/快手/小红书/闲鱼/TikTok）
 * 3. tagType 对齐 Element Plus el-tag
 * 4. group 用于按场景过滤（card：卡片；clue：线索；account：账号）
 *
 * 维护规则：新增/修改卡片平台只需修改本文件即可全局生效。
 */

export const CARD_PLATFORM = Object.freeze({
  DOUYIN: 'douyin',           // 抖音
  KUAISHOU: 'kuaishou',       // 快手
  XIAOHONGSHU: 'xiaohongshu', // 小红书
  XIANYU: 'xianyu',           // 闲鱼
  TIKTOK: 'tiktok'            // TikTok
})

// 完整卡片平台列表
export const CARD_PLATFORM_OPTIONS = Object.freeze([
  { value: 'douyin',       label: '抖音',   tagType: '',        sort: 1, group: 'card',   icon: 'VideoCamera' },
  { value: 'kuaishou',     label: '快手',   tagType: '',        sort: 2, group: 'card',   icon: 'VideoCamera' },
  { value: 'xiaohongshu',  label: '小红书', tagType: 'danger',  sort: 3, group: 'card',   icon: 'Postcard' },
  { value: 'xianyu',       label: '闲鱼',   tagType: 'warning', sort: 4, group: 'card',   icon: 'Goods' },
  { value: 'tiktok',       label: 'TikTok', tagType: '',        sort: 5, group: 'card',   icon: 'VideoCamera' }
])

// 与 channel.js CHANNEL_OPTIONS 兼容的线索类型平台（1=小红书 2=视频号 3=抖音 4=快手）
// 原 src/utils/map.js 中 getClueMap/getClueName 使用的 1-4 数字值
export const CLUE_TYPE_OPTIONS = Object.freeze([
  { value: '1', label: '小红书', tagType: 'danger',  sort: 1, group: 'clue' },
  { value: '2', label: '视频号', tagType: 'success', sort: 2, group: 'clue' },
  { value: '3', label: '抖音',   tagType: '',        sort: 3, group: 'clue' },
  { value: '4', label: '快手',   tagType: '',        sort: 4, group: 'clue' }
])

// 旧 utils/map.js 兼容的 1-6 线索类型（含 QQ/微信/电话/Telegram/WhatsApp/twitter）
export const CLUE_TYPE_OPTIONS_LEGACY = Object.freeze([
  { value: '1', label: 'QQ',         tagType: 'info', sort: 1, group: 'legacy' },
  { value: '2', label: '微信',       tagType: 'success', sort: 2, group: 'legacy' },
  { value: '3', label: '电话',       tagType: 'warning', sort: 3, group: 'legacy' },
  { value: '4', label: 'Telegram',   tagType: 'primary', sort: 4, group: 'legacy' },
  { value: '5', label: 'Whatsapp',   tagType: 'success', sort: 5, group: 'legacy' },
  { value: '6', label: 'twitter',    tagType: 'info', sort: 6, group: 'legacy' }
])

// value -> label 快查（合并所有平台/线索 type）
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

export const CARD_PLATFORM_LABEL_MAP = Object.freeze(_buildLabelMap(CARD_PLATFORM_OPTIONS))
export const CARD_PLATFORM_TAG_TYPE_MAP = Object.freeze(_buildTagTypeMap(CARD_PLATFORM_OPTIONS))
export const CLUE_TYPE_LABEL_MAP = Object.freeze({
  ..._buildLabelMap(CLUE_TYPE_OPTIONS),
  ..._buildLabelMap(CLUE_TYPE_OPTIONS_LEGACY)
})

/**
 * 获取卡片平台中文 label（兼容 1-4 数字与 douyin/kuaishou 字符串）
 * @param {*} value
 * @returns {string} label；找不到时回退 value
 */
export const getCardPlatformLabel = (value) => {
  if (value === undefined || value === null || value === '') return '-'
  return CARD_PLATFORM_LABEL_MAP[value]
    || CLUE_TYPE_LABEL_MAP[value]
    || String(value)
}

/**
 * 获取卡片平台 el-tag 类型
 * @param {*} value
 * @returns {string}
 */
export const getCardPlatformTagType = (value) => {
  if (value === undefined || value === null || value === '') return ''
  return CARD_PLATFORM_TAG_TYPE_MAP[value] || CLUE_TYPE_LABEL_MAP[value] || ''
}

/**
 * 获取线索类型 label（兼容 1-4 数字与 douyin/kuaishou 字符串）
 * 原 getClueName 替代品
 * @param {*} value
 * @returns {string}
 */
export const getClueTypeLabel = (value) => {
  if (value === undefined || value === null || value === '') return '-'
  return CLUE_TYPE_LABEL_MAP[value] || CARD_PLATFORM_LABEL_MAP[value] || String(value)
}

/**
 * 获取线索类型 options 列表
 * 原 getClueMap 替代品
 * @returns {Array}
 */
export const getClueTypeOptions = () => CLUE_TYPE_OPTIONS

/**
 * 按 group 过滤卡片平台列表
 * @param {string|string[]} groups
 * @returns {Array}
 */
export const filterCardPlatformByGroup = (groups) => {
  const list = Array.isArray(groups) ? groups : [groups]
  return [
    ...CARD_PLATFORM_OPTIONS,
    ...CLUE_TYPE_OPTIONS,
    ...CLUE_TYPE_OPTIONS_LEGACY
  ].filter((o) => list.includes(o.group))
}

export default {
  CARD_PLATFORM,
  CARD_PLATFORM_OPTIONS,
  CLUE_TYPE_OPTIONS,
  CLUE_TYPE_OPTIONS_LEGACY,
  CARD_PLATFORM_LABEL_MAP,
  CARD_PLATFORM_TAG_TYPE_MAP,
  CLUE_TYPE_LABEL_MAP,
  getCardPlatformLabel,
  getCardPlatformTagType,
  getClueTypeLabel,
  getClueTypeOptions,
  filterCardPlatformByGroup
}
