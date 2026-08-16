/**
 * i18n 复数规则工具（USR-I18N-06）
 * 借鉴：vue-i18n 9 + ICU MessageFormat
 * 复数规则：俄/阿/波兰有 3-4 形（one/few/many/other）
 */

const PLURAL_RULES = {
  zh: { categories: ['other'], select: (n) => 'other' },
  en: { categories: ['one', 'other'], select: (n) => (n === 1 ? 'one' : 'other') },
  ja: { categories: ['other'], select: (n) => 'other' },
  ar: {
    categories: ['zero', 'one', 'two', 'few', 'many', 'other'],
    select: (n) => {
      if (n === 0) return 'zero'
      if (n === 1) return 'one'
      if (n === 2) return 'two'
      const mod100 = n % 100
      if (mod100 >= 3 && mod100 <= 10) return 'few'
      if (mod100 >= 11 && mod100 <= 99) return 'many'
      return 'other'
    }
  },
  ru: {
    categories: ['one', 'few', 'many', 'other'],
    select: (n) => {
      const mod10 = n % 10
      const mod100 = n % 100
      if (mod10 === 1 && mod100 !== 11) return 'one'
      if (mod10 >= 2 && mod10 <= 4 && (mod100 < 10 || mod100 >= 20)) return 'few'
      if (mod10 === 0 || (mod10 >= 5 && mod10 <= 9) || (mod100 >= 11 && mod100 <= 14)) return 'many'
      return 'other'
    }
  },
  fr: {
    categories: ['one', 'many', 'other'],
    select: (n) => (n === 0 || n === 1 ? 'one' : 'many')
  },
  de: { categories: ['one', 'other'], select: (n) => (n === 1 ? 'one' : 'other') },
  es: { categories: ['one', 'other'], select: (n) => (n === 1 ? 'one' : 'other') },
  pt: { categories: ['one', 'other'], select: (n) => (n === 1 ? 'one' : 'other') }
}

/**
 * 选择复数形式
 * @param {string} locale
 * @param {number} n
 * @returns {string} 'one' | 'few' | 'many' | 'other' | ...
 */
export const pluralSelect = (locale, n) => {
  const rule = PLURAL_RULES[locale] || PLURAL_RULES.en
  return rule.select(n)
}

/**
 * 复数化消息
 * @param {object} messages - 形如 { one: '1 item', other: '# items' }
 * @param {number} n
 * @param {string} locale
 * @returns {string}
 */
export const plural = (messages, n, locale) => {
  const cat = pluralSelect(locale, n)
  // 优先匹配该类别
  if (messages[cat]) return messages[cat].replace(/#/g, n)
  // 回退到 other
  if (messages.other) return messages.other.replace(/#/g, n)
  return messages.one ? messages.one.replace(/#/g, n) : String(n)
}

/**
 * ICU MessageFormat 简化版
 * @param {string} template - 例: "{count, plural, one {# item} other {# items}}"
 * @param {object} vars - 形如 { count: 5 }
 * @param {string} locale
 */
export const icuPlural = (template, vars, locale) => {
  // 简化：匹配 {var, plural, ... }
  const re = /\{(\w+),\s*plural,\s*([^}]+(?:\{[^}]*\}[^}]*)*)\}/g
  return template.replace(re, (_, varName, pluralBody) => {
    const n = vars[varName]
    if (typeof n !== 'number') return ''
    const cat = pluralSelect(locale, n)
    // 解析 pluralBody 找到匹配的类别
    const parts = pluralBody.split(/(\w+)\s*\{/).filter(Boolean)
    let selectedBody = null
    let i = 0
    while (i < parts.length) {
      const cat = parts[i].trim()
      if (cat && parts[i + 1]) {
        // 找匹配的右大括号
        let depth = 1
        let body = ''
        const rest = parts[i + 1]
        for (const ch of rest) {
          if (ch === '{') depth++
          if (ch === '}') {
            depth--
            if (depth === 0) break
          }
          body += ch
        }
        if (cat === pluralSelect(locale, n)) {
          selectedBody = body
          break
        }
      }
      i += 2
    }
    if (!selectedBody) selectedBody = '#'
    return selectedBody.replace(/#/g, n)
  })
}

export default { pluralSelect, plural, icuPlural, PLURAL_RULES }
