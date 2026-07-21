// 多语言核心：支持语言列表、持久化、RTL（阿拉伯语从右向左）方向切换。
// 支持：简体中文(zh) / English(en) / 日本語(ja) / العربية(ar)

export const SUPPORTED_LOCALES = [
  { code: 'zh', label: '简体中文', el: 'zh-cn', rtl: false },
  { code: 'en', label: 'English', el: 'en', rtl: false },
  { code: 'ja', label: '日本語', el: 'ja', rtl: false },
  // Element Plus 暂未提供 ar 语言包，组件内置文案回退英文，自有文案仍走阿拉伯语
  { code: 'ar', label: 'العربية', el: 'en', rtl: true },
]

const STORAGE_KEY = 'app_locale'

export function isSupported(code) {
  return SUPPORTED_LOCALES.some((l) => l.code === code)
}

export function getStoredLocale() {
  const v = localStorage.getItem(STORAGE_KEY)
  if (v && isSupported(v)) return v
  // 默认跟随浏览器语言
  const nav = (navigator.language || 'zh').toLowerCase()
  const base = nav.split('-')[0]
  if (base === 'en') return 'en'
  if (base === 'ja') return 'ja'
  if (base === 'ar') return 'ar'
  return 'zh'
}

// 设置语言并持久化 + 应用 RTL/LTR 方向（阿拉伯语需要 dir=rtl）
export function setLocale(locale) {
  if (!isSupported(locale)) locale = 'zh'
  localStorage.setItem(STORAGE_KEY, locale)
  applyDirection(locale)
  return locale
}

// 应用文档方向，供阿拉伯语 RTL 排版
export function applyDirection(locale) {
  const item = SUPPORTED_LOCALES.find((l) => l.code === locale)
  const dir = item && item.rtl ? 'rtl' : 'ltr'
  document.documentElement.setAttribute('dir', dir)
  document.documentElement.setAttribute('lang', locale)
}

// 返回当前语言对应的 Element Plus 语言包标识（用于 el-config-provider）
export function elLocaleCode(locale) {
  const item = SUPPORTED_LOCALES.find((l) => l.code === locale)
  return item ? item.el : 'zh-cn'
}
