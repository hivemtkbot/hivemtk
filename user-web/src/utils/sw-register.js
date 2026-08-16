/**
 * Service Worker 注册封装（vite-plugin-pwa runtime）
 *
 * 任务编号：OPT-FE-12
 * 行为：
 *   - 生产构建后由 vite-plugin-pwa 注入 registerSW.js
 *   - 仅在 import.meta.env.PROD=true 时注册，避免 dev 模式刷缓存
 *   - 监听 onNeedRefresh / onOfflineReady，向全局派发 CustomEvent，
 *     由 src/components/PwaUpdatePrompt.vue 监听并提示用户
 */

const NEED_REFRESH_EVENT = 'pwa:need-refresh'
const OFFLINE_READY_EVENT = 'pwa:offline-ready'

export async function registerServiceWorker() {
  if (typeof window === 'undefined') return
  if (!('serviceWorker' in navigator)) return
  if (!import.meta.env.PROD) {
    // 开发模式：跳过注册，避免 vite HMR 与 SW 缓存冲突
    return
  }

  try {
    // vite-plugin-pwa 在生产构建时生成 /registerSW.js
    const { registerSW } = await import('virtual:pwa-register')
    const updateSW = registerSW({
      immediate: true,
      onNeedRefresh() {
        window.dispatchEvent(new CustomEvent(NEED_REFRESH_EVENT))
      },
      onOfflineReady() {
        window.dispatchEvent(new CustomEvent(OFFLINE_READY_EVENT))
      },
      onRegisteredSW(swUrl) {
        // eslint-disable-next-line no-console
        console.info('[pwa] service worker registered at', swUrl)
      },
      onRegisterError(err) {
        console.error('[pwa] service worker register failed', err)
      },
    })
    // 提供手动更新入口，挂到 window 以便外部触发（如设置页"检查更新"按钮）
    window.__pwaUpdate = () => updateSW(true)
  } catch (e) {
    // virtual:pwa-register 不可用（dev / 未配置）时静默回退
    if (import.meta.env.DEV) return
    console.warn('[pwa] registerSW skipped:', e && e.message)
  }
}

export const PWA_EVENTS = Object.freeze({
  NEED_REFRESH: NEED_REFRESH_EVENT,
  OFFLINE_READY: OFFLINE_READY_EVENT,
})
