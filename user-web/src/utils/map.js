import { getClueTypeOptions, CLUE_TYPE_OPTIONS_LEGACY } from '@/constants/cardPlatform';
import { getOrderStatusLabel, getOrderStatusTagType } from '@/constants/orderStatus'

const _LEGACY_PLATFORM_NAME_MAP = {
  '1': '小红书',
  '2': '视频号',
  '3': '抖音',
  '4': '快手'
};
const _LEGACY_PLATFORM_TAG_MAP = {
  '1': 'success',
  '2': 'success',
  '3': 'warning',
  '4': 'info'
}

export const getPlatformName = (type) => {
  if (type === undefined || type === null || type === '') return '未知'
  return _LEGACY_PLATFORM_NAME_MAP[String(type)] ?? '未知'
};

export const getPlatformTag = (type) => {
  if (type === undefined || type === null || type === '') return '未知'
  return _LEGACY_PLATFORM_TAG_MAP[String(type)] ?? '未知'
};

export const getPlatformMap = () => getClueTypeOptions();

const _LEGACY_STATUS_TYPE_MAP = {
  '1': 'info',
  '5': 'warning',
  '8': 'success',
  '9': 'info',
  '10': 'danger'
};
const _LEGACY_STATUS_NAME_MAP = {
  '1': '待执行',
  '5': '进行中',
  '8': '已失败',
  '9': '已取消',
  '10': '已完成'
}

export const getStatusTag = (status) => _LEGACY_STATUS_TYPE_MAP[status] || getOrderStatusTagType(status) || 'info';

export const getStatusName = (status) => _LEGACY_STATUS_NAME_MAP[status] || getOrderStatusLabel(status) || '未知';

export const getClueMap = () => CLUE_TYPE_OPTIONS_LEGACY.slice();

export const getClueName = (type) => {
  if (type === undefined || type === null || type === '') return '未知'
  const legacy = { '1': 'QQ', '2': '微信', '3': '电话', '4': 'Telegram', '5': 'Whatsapp', '6': 'twitter' };
  return legacy[String(type)] || '未知'
};
