/**
 * src/utils/map.js — 旧版平台/状态/线索类型映射（deprecated）
 *
 * 本文件保留仅为向后兼容。所有新代码应使用 src/constants/ 下的统一枚举：
 * - 平台/卡片  -> @/constants/cardPlatform  (getCardPlatformLabel, getClueTypeLabel, getClueTypeOptions)
 * - 线索状态   -> @/constants/leadStatus    (getLeadStatusLabel, getLeadStatusTagType)
 * - 客户状态   -> @/constants/customerTag   (getCustomerStatusLabel, getCustomerStatusTagType)
 * - 账号状态   -> @/constants/accountType   (getAccountStatusLabel, getAccountStatusTagType)
 * - 订单/作业  -> @/constants/orderStatus    (getOrderStatusLabel, getOrderStatusTagType)
 * - 渠道       -> @/constants/channel       (getChannelLabel, getChannelTagType)
 *
 * 本文件所有 API 委托转发到 constants，行为完全等价，
 * 但已弃用，请勿在新代码中继续 import '@/utils/map'。
 */

import { getClueTypeOptions, CLUE_TYPE_OPTIONS_LEGACY } from '@/constants/cardPlatform'
import { getOrderStatusLabel, getOrderStatusTagType } from '@/constants/orderStatus'

// ============ Deprecated: 平台相关 ============
// 旧版平台类型（1=小红书 2=视频号 3=抖音 4=快手），与 channel.js 兼容的线索平台。
// 注意：此处与 cardPlatform.js 的 CLUE_TYPE_OPTIONS_LEGACY（1=QQ/2=微信/…）是两套不同的 1-6 体系，
// 二者在 1-4 上语义冲突，故 deprecated 函数保留独立映射，确保历史行为稳定，不被 channel 映射覆盖。
const _LEGACY_PLATFORM_NAME_MAP = {
  '1': '小红书',
  '2': '视频号',
  '3': '抖音',
  '4': '快手'
}
const _LEGACY_PLATFORM_TAG_MAP = {
  '1': 'success',
  '2': 'success',
  '3': 'warning',
  '4': 'info'
}

/**
 * @deprecated 使用 @/constants/cardPlatform#getCardPlatformLabel 代替
 * 旧版 1-4 平台中文名：1=小红书 2=视频号 3=抖音 4=快手；未知返回 "未知"
 */
export const getPlatformName = (type) => {
  if (type === undefined || type === null || type === '') return '未知'
  return _LEGACY_PLATFORM_NAME_MAP[String(type)] ?? '未知'
}

/**
 * @deprecated 使用 @/constants/cardPlatform#getCardPlatformTagType 代替
 * 旧版 1-4 平台标签：1=success 4=info；未知返回 "未知"
 */
export const getPlatformTag = (type) => {
  if (type === undefined || type === null || type === '') return '未知'
  return _LEGACY_PLATFORM_TAG_MAP[String(type)] ?? '未知'
}

/**
 * @deprecated 使用 @/constants/cardPlatform#getClueTypeOptions 代替
 */
export const getPlatformMap = () => getClueTypeOptions()

// ============ Deprecated: 状态相关（来自 src/utils/map.js 旧版 getStatusTag/getStatusName）============
// 旧版 map.js 的 1/5/8/9/10 状态值已不存在于新业务中；保留仅做签名兼容。
const _LEGACY_STATUS_TYPE_MAP = {
  '1': 'info',
  '5': 'warning',
  '8': 'success',
  '9': 'info',
  '10': 'danger'
}
const _LEGACY_STATUS_NAME_MAP = {
  '1': '待执行',
  '5': '进行中',
  '8': '已失败',
  '9': '已取消',
  '10': '已完成'
}

/**
 * @deprecated 原 1/5/8/9/10 数字状态已废弃；请使用 @/constants/orderStatus#getOrderStatusTagType
 */
export const getStatusTag = (status) => _LEGACY_STATUS_TYPE_MAP[status] || getOrderStatusTagType(status) || 'info'

/**
 * @deprecated 原 1/5/8/9/10 数字状态已废弃；请使用 @/constants/orderStatus#getOrderStatusLabel
 */
export const getStatusName = (status) => _LEGACY_STATUS_NAME_MAP[status] || getOrderStatusLabel(status) || '未知'

// ============ Deprecated: 线索类型相关（QQ/微信/电话/... 1-6 数字）============
// 旧版 map.js 的 1-6 数字线索类型是历史实现，迁移到 cardPlatform.js 的 CLUE_TYPE_OPTIONS_LEGACY
// 保留 1-4 与新 CLUE_TYPE_OPTIONS 一致（小红书/视频号/抖音/快手）

/**
 * @deprecated 使用 @/constants/cardPlatform#getClueTypeOptions 代替
 */
export const getClueMap = () => CLUE_TYPE_OPTIONS_LEGACY.slice()

/**
 * @deprecated 使用 @/constants/cardPlatform#getClueTypeLabel 代替
 */
export const getClueName = (type) => {
  if (type === undefined || type === null || type === '') return '未知'
  // 1-6 数字映射（与原 utils/map.js 行为一致）：1=QQ 2=微信 3=电话 4=Telegram 5=Whatsapp 6=twitter
  const legacy = { '1': 'QQ', '2': '微信', '3': '电话', '4': 'Telegram', '5': 'Whatsapp', '6': 'twitter' }
  return legacy[String(type)] || '未知'
}
