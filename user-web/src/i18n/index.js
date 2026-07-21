// vue-i18n 实例构建。
// 业务模块语言文件放在 ./modules/*.js，每个文件默认导出一个形如
//   { zh: {...}, en: {...}, ja: {...}, ar: {...} } 的对象，键名需带命名空间（如 chat.send）。
// 通过 import.meta.glob 自动收集，新增模块无需修改本文件，便于并行协作。
import { createI18n } from 'vue-i18n'
import { getStoredLocale, applyDirection } from './locale'

const modules = import.meta.glob('./modules/*.js', { eager: true })

const messages = { zh: {}, en: {}, ja: {}, ar: {} }
for (const path in modules) {
  const mod = modules[path].default || modules[path]
  for (const lang of ['zh', 'en', 'ja', 'ar']) {
    if (mod && mod[lang]) {
      messages[lang] = { ...messages[lang], ...mod[lang] }
    }
  }
}

const locale = getStoredLocale()
applyDirection(locale)

const i18n = createI18n({
  legacy: false, // 组合式 API 模式
  locale,
  fallbackLocale: 'zh',
  messages,
  missingWarn: false,
  fallbackWarn: false,
})

export default i18n
