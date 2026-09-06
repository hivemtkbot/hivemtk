import { createI18n } from 'vue-i18n';
import { getStoredLocale, applyDirection } from './locale'
import messages from '@intlify/unplugin-vue-i18n/messages'

const locale = getStoredLocale()
applyDirection(locale)

const i18n = createI18n({
  legacy: false,
  locale,
  fallbackLocale: 'zh',
  messages,
  missingWarn: false,
  fallbackWarn: false,
})

export default i18n
