const NEED_REFRESH_EVENT = 'pwa:need-refresh';
const OFFLINE_READY_EVENT = 'pwa:offline-ready'

export async function registerServiceWorker() {
  if (typeof window === 'undefined') return
  if (!('serviceWorker' in navigator)) return
  if (!import.meta.env.PROD) {
    return;
  }

  try {
    const { registerSW } = await import('virtual:pwa-register');
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
    window.__pwaUpdate = () => updateSW(true);
  } catch (e) {
    if (import.meta.env.DEV)
      return;
    console.warn('[pwa] registerSW skipped:', e && e.message)
  }
}

export const PWA_EVENTS = Object.freeze({
  NEED_REFRESH: NEED_REFRESH_EVENT,
  OFFLINE_READY: OFFLINE_READY_EVENT,
})
