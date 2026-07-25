/**
 * 统一多语言枚举：智能体/渠道 出海语言配置
 *
 * 与后端 user-server/internal/pkg/i18n/lang_ctx.go 中 SupportedLanguages 保持一致。
 * 用于：
 *   - 智能体管理：internal_language（内部语言）/ target_language（目标语言）
 *   - 渠道管理：target_language（目标语言，覆盖智能体配置）
 *
 * 双语言模型：
 *   - 内部语言：商户维护知识库使用的语言，影响内部工具处理精度。默认 zh。
 *   - 目标语言：智能体/渠道对外输出语言。空表示跟随内部语言（同语种零开销）。
 *
 * 用法：
 *   import { SUPPORTED_LANGUAGES, INTERNAL_LANGUAGE_OPTIONS, TARGET_LANGUAGE_OPTIONS, getLanguageLabel } from '@/constants/languages'
 */

// 支持的语言列表（与后端 i18n.SupportedLanguages 一致，ISO 639-1 短码）
export const SUPPORTED_LANGUAGES = Object.freeze([
  { value: 'zh', label: '简体中文' },
  { value: 'en', label: 'English' },
  { value: 'ja', label: '日本語' },
  { value: 'ko', label: '한국어' },
  { value: 'de', label: 'Deutsch' },
  { value: 'fr', label: 'Français' },
  { value: 'es', label: 'Español' },
  { value: 'pt', label: 'Português' },
  { value: 'ru', label: 'Русский' },
  { value: 'ar', label: 'العربية' },
  { value: 'it', label: 'Italiano' },
  { value: 'nl', label: 'Nederlands' },
  { value: 'th', label: 'ไทย' },
  { value: 'vi', label: 'Tiếng Việt' },
  { value: 'id', label: 'Bahasa Indonesia' },
  { value: 'tr', label: 'Türkçe' },
  { value: 'pl', label: 'Polski' },
  { value: 'hi', label: 'हिन्दी' }
])

// 内部语言选项（含"默认中文"语义提示，zh 置顶且标注默认）
export const INTERNAL_LANGUAGE_OPTIONS = Object.freeze([
  { value: 'zh', label: '简体中文（默认）' },
  ...SUPPORTED_LANGUAGES.filter((l) => l.value !== 'zh')
])

// 目标语言选项（含"跟随内部语言"空值选项，置于首位）
export const TARGET_LANGUAGE_OPTIONS = Object.freeze([
  { value: '', label: '跟随智能体内部语言' },
  ...SUPPORTED_LANGUAGES
])

// value -> label 快查
export const LANGUAGE_LABEL_MAP = Object.freeze(
  SUPPORTED_LANGUAGES.reduce((acc, o) => { acc[o.value] = o.label; return acc }, {})
)

/**
 * 根据语言代码获取语言名称
 * @param {string} code 语言代码，空串/undefined/null 返回"跟随内部语言"
 * @returns {string}
 */
export const getLanguageLabel = (code) => {
  if (code === undefined || code === null || code === '') return '跟随内部语言'
  return LANGUAGE_LABEL_MAP[code] || String(code)
}

export default {
  SUPPORTED_LANGUAGES,
  INTERNAL_LANGUAGE_OPTIONS,
  TARGET_LANGUAGE_OPTIONS,
  LANGUAGE_LABEL_MAP,
  getLanguageLabel
}
