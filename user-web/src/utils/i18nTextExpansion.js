const EXPANSION_FACTORS = {
  zh: 1.0,
  en: 1.5,
  ja: 1.2,
  ar: 1.4,
  es: 1.6,
  fr: 1.6,
  de: 1.7,
  ru: 1.5,
  pt: 1.5
};

export const getExpansionFactor = (locale) => {
  if (typeof localStorage !== 'undefined') {
    const v = localStorage.getItem('app_locale')
    if (v) return EXPANSION_FACTORS[v] || 1.5
  }
  return 1.5
};

export const calcButtonMinWidth = (text, locale, fontFamily = 'Inter') => {
  if (!text) return '0'
  const charWidth = locale === 'zh' || locale === 'ja' ? 14 : 8;
  const padding = 32;
  const factor = getExpansionFactor(locale)
  const baseWidth = text.length * charWidth + padding
  return `${Math.ceil(baseWidth * factor)}px`
};

export const willOverflow = (text, maxWidth = 120) => {
  if (!text) return false
  const textWidth = text.length * 8 * 1.5
  return textWidth > maxWidth
};

export const MIN_BUTTON_WIDTH = {
  short: { zh: 60, en: 80, ja: 70, ar: 75, default: 80 },
  medium: { zh: 90, en: 120, ja: 100, ar: 110, default: 110 },
  long: { zh: 120, en: 180, ja: 140, ar: 160, default: 160 }
};

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
