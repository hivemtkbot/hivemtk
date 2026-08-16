/**
 * i18n 文本膨胀处理工具（USR-I18N-02）
 * 借鉴：https://www.autolocalise.com/blog/ui-localization-best-practices
 *
 * 中文 4 字符 → 阿拉伯 12 字符 → 英语 25 字符
 * 设计按钮/卡片/导航时按 +50% 文本膨胀
 */

const EXPANSION_FACTORS = {
  zh: 1.0,    // 中文（基准）
  en: 1.5,    // 英文
  ja: 1.2,    // 日文
  ar: 1.4,    // 阿拉伯
  es: 1.6,    // 西班牙
  fr: 1.6,    // 法语
  de: 1.7,    // 德语
  ru: 1.5,    // 俄语
  pt: 1.5     // 葡萄牙
}

/**
 * 获取当前 locale 的膨胀系数
 */
export const getExpansionFactor = (locale) => {
  if (typeof localStorage !== 'undefined') {
    const v = localStorage.getItem('app_locale')
    if (v) return EXPANSION_FACTORS[v] || 1.5
  }
  return 1.5
}

/**
 * 计算按钮应预留的最小宽度
 * @param {string} text 文本
 * @param {string} locale 语言
 * @param {string} fontFamily
 * @returns {string} CSS min-width
 */
export const calcButtonMinWidth = (text, locale, fontFamily = 'Inter') => {
  if (!text) return '0'
  // 简化的中文字符宽度
  const charWidth = locale === 'zh' || locale === 'ja' ? 14 : 8
  const padding = 32 // 左右 padding
  const factor = getExpansionFactor(locale)
  const baseWidth = text.length * charWidth + padding
  return `${Math.ceil(baseWidth * factor)}px`
}

/**
 * 检查按钮文本是否会膨胀超限
 * @param {string} text
 * @param {number} maxWidth
 */
export const willOverflow = (text, maxWidth = 120) => {
  if (!text) return false
  const textWidth = text.length * 8 * 1.5
  return textWidth > maxWidth
}

/**
 * 建议最小宽度（避免文字截断）
 */
export const MIN_BUTTON_WIDTH = {
  short: { zh: 60, en: 80, ja: 70, ar: 75, default: 80 },
  medium: { zh: 90, en: 120, ja: 100, ar: 110, default: 110 },
  long: { zh: 120, en: 180, ja: 140, ar: 160, default: 160 }
}

export const getMinWidth = (size, locale) => {
  const config = MIN_BUTTON_WIDTH[size] || MIN_BUTTON_WIDTH.medium
  return config[locale] || config.default
}

export default {
  getExpansionFactor,
  calcButtonMinWidth,
  willOverflow,
  getMinWidth,
  EXPANSION_FACTORS
}
