// vue-i18n 实例构建。
// 语言资源放在 ./locales/<locale>.json（zh/en/ja/ar），由 @intlify/unplugin-vue-i18n
// 在构建期预编译为运行时函数，从而消除运行期 new Function 编译消息导致的
// CSP unsafe-eval 违规（script-src 'self' 会拦截 eval，致含 i18n 的页面整页白屏）。
// 新增/修改文案直接编辑对应 locale 的 JSON 文件即可，无需改动本文件。
import { createI18n } from 'vue-i18n'
import { getStoredLocale, applyDirection } from './locale'
import messages from '@intlify/unplugin-vue-i18n/messages'

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
