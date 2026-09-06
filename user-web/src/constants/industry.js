/**
 * 统一通用枚举：行业（industry）
 *
 * 业务：assetMarket 资产市场 / assetBundle 资产包
 * 与 user-server internal/domain/asset/entity.go Industry 枚举严格对齐
 */

export const INDUSTRY_OPTIONS = Object.freeze([
  { value: '美妆', label: '美妆' },
  { value: '教培', label: '教培' },
  { value: '医美', label: '医美' },
  { value: '汽车', label: '汽车' },
  { value: '金融', label: '金融' },
  { value: '电子烟', label: '电子烟' },
  { value: '成人用品', label: '成人用品' },
  { value: '两性健康', label: '两性健康' },
  { value: '租车', label: '租车' },
  { value: '民宿', label: '民宿' },
  { value: '货代', label: '货代' },
  { value: '移民', label: '移民' }
])

export const INDUSTRY_VALUES = Object.freeze(INDUSTRY_OPTIONS.map(o => o.value))

export const getIndustryLabel = (v) => {
  if (v === undefined || v === null || v === '') return '-'
  return v
}

export default {
  INDUSTRY_OPTIONS,
  INDUSTRY_VALUES,
  getIndustryLabel
}
