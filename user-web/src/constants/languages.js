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
]);

export const INTERNAL_LANGUAGE_OPTIONS = Object.freeze([
  { value: 'zh', label: '简体中文（默认）' },
  ...SUPPORTED_LANGUAGES.filter((l) => l.value !== 'zh')
]);

export const TARGET_LANGUAGE_OPTIONS = Object.freeze([
  { value: '', label: '跟随智能体内部语言' },
  ...SUPPORTED_LANGUAGES
]);

export const LANGUAGE_LABEL_MAP = Object.freeze(
  SUPPORTED_LANGUAGES.reduce((acc, o) => { acc[o.value] = o.label; return acc }, {})
);

export const getLanguageLabel = (code) => {
  if (code === undefined || code === null || code === '') return '跟随内部语言'
  return LANGUAGE_LABEL_MAP[code] || String(code)
};

export default {
  SUPPORTED_LANGUAGES,
  INTERNAL_LANGUAGE_OPTIONS,
  TARGET_LANGUAGE_OPTIONS,
  LANGUAGE_LABEL_MAP,
  getLanguageLabel
}
