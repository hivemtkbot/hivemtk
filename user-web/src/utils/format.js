const NUMBER_FORMAT_CACHE = new Map();
const DATE_FORMAT_CACHE = new Map()

export const getLocale = () => {
  if (typeof localStorage !== 'undefined') {
    const v = localStorage.getItem('app_locale')
    if (v) return v
  }
  return (typeof navigator !== 'undefined' && navigator.language) || 'zh'
};

const toIntlLocale = (locale) => {
  const map = {
    zh: 'zh-CN',
    en: 'en-US',
    ja: 'ja-JP',
    ar: 'ar-EG',
    es: 'es-ES',
    fr: 'fr-FR',
    de: 'de-DE',
    ru: 'ru-RU',
    pt: 'pt-PT'
  }
  return map[locale] || locale
};

const cacheKey = (locale, type, opts) => `${toIntlLocale(locale)}|${type}|${JSON.stringify(opts)}`

export const formatNumber = (num, locale = getLocale(), opts = {}) => {
  const key = cacheKey(locale, 'number', opts)
  if (!NUMBER_FORMAT_CACHE.has(key)) {
    NUMBER_FORMAT_CACHE.set(key, new Intl.NumberFormat(toIntlLocale(locale), opts))
  }
  return NUMBER_FORMAT_CACHE.get(key).format(num)
};

export const formatCurrency = (amount, currency = 'CNY', locale = getLocale()) => {
  const opts = { style: 'currency', currency }
  const key = cacheKey(locale, 'currency', opts)
  if (!NUMBER_FORMAT_CACHE.has(key)) {
    NUMBER_FORMAT_CACHE.set(key, new Intl.NumberFormat(toIntlLocale(locale), opts))
  }
  return NUMBER_FORMAT_CACHE.get(key).format(amount)
};

export const formatPercent = (num, locale = getLocale(), fractionDigits = 2) => {
  return formatNumber(num / 100, locale, {
    style: 'percent',
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits
  })
};

export const formatDate = (date, locale = getLocale(), opts = { dateStyle: 'medium' }) => {
  const key = cacheKey(locale, 'date', opts)
  if (!DATE_FORMAT_CACHE.has(key)) {
    DATE_FORMAT_CACHE.set(key, new Intl.DateTimeFormat(toIntlLocale(locale), opts))
  }
  const d = date instanceof Date ? date : new Date(date)
  if (isNaN(d.getTime())) return ''
  return DATE_FORMAT_CACHE.get(key).format(d)
};

export const formatRelativeTime = (date, locale = getLocale()) => {
  const d = date instanceof Date ? date : new Date(date)
  if (isNaN(d.getTime())) return ''
  const diffMs = d.getTime() - Date.now()
  const diffSec = Math.round(diffMs / 1000)
  const rtf = new Intl.RelativeTimeFormat(toIntlLocale(locale), { numeric: 'auto' })
  const abs = Math.abs(diffSec)
  if (abs < 60) return rtf.format(diffSec, 'second')
  if (abs < 3600) return rtf.format(Math.round(diffSec / 60), 'minute')
  if (abs < 86400) return rtf.format(Math.round(diffSec / 3600), 'hour')
  if (abs < 2592000) return rtf.format(Math.round(diffSec / 86400), 'day')
  if (abs < 31536000) return rtf.format(Math.round(diffSec / 2592000), 'month')
  return rtf.format(Math.round(diffSec / 31536000), 'year')
};

export const formatList = (items, locale = getLocale(), opts = { type: 'conjunction', style: 'long' }) => {
  return new Intl.ListFormat(toIntlLocale(locale), opts).format(items)
};

export const formatCompactNumber = (num, locale = getLocale()) => {
  return formatNumber(num, locale, { notation: 'compact', maximumFractionDigits: 1 })
};

export const formatFileSize = (bytes, locale = getLocale()) => {
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let i = -1
  let n = bytes
  do {
    n /= 1024
    i++
  } while (n >= 1024 && i < units.length - 1)
  return formatNumber(n, locale, { maximumFractionDigits: 1 }) + ' ' + units[i]
};

export default {
  formatNumber,
  formatCurrency,
  formatPercent,
  formatDate,
  formatRelativeTime,
  formatList,
  formatCompactNumber,
  formatFileSize,
  getLocale
}
