import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import pinia from './stores'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import './styles/index.scss'
import { updateRequestConfig, http } from './utils/request'
import i18n from './i18n'
import { applyDirection } from './i18n/locale'

applyDirection(i18n.global.locale.value);

const app = createApp(App)

app.use(router)
app.use(pinia)
app.use(ElementPlus)
app.use(i18n)

updateRequestConfig().catch(error => {
  console.error('初始化API配置失败:', error)
});

router.afterEach((to) => {
  const cspMeta = document.querySelector('meta[http-equiv="Content-Security-Policy"]')
  if (!cspMeta) return

  if (to.path.startsWith('/chat/embed')) {
    const csp = cspMeta.getAttribute('content');
    const newCsp = csp.replace(/frame-ancestors\s+[^;]+/, "frame-ancestors *")
    cspMeta.setAttribute('content', newCsp)
  } else {
    const csp = cspMeta.getAttribute('content');
    if (csp && !csp.includes("frame-ancestors 'none'")) {
      const newCsp = csp.replace(/frame-ancestors\s+[^;]+/, "frame-ancestors 'none'")
      cspMeta.setAttribute('content', newCsp)
    }
  }
});

app.mount('#app')

const SW_VERSION_KEY = 'hivemtk-build-id';
const SW_REGISTERED_KEY = 'hivemtk-sw-registered'

async function manageServiceWorker() {
  if (!('serviceWorker' in navigator)) {
    console.info('[SW] serviceWorker 不可用, 跳过注册')
    return
  }

  try {
    const resp = await fetch('/version.json', { cache: 'no-store' });
    if (!resp.ok) {
      console.warn(`[SW] 拉 version.json 失败: HTTP ${resp.status}`)
      return
    }
    const remoteVersion = await resp.json()
    const remoteBuildId = remoteVersion.buildId

    const localBuildId = localStorage.getItem(SW_VERSION_KEY);

    if (localBuildId && localBuildId !== remoteBuildId) {
      console.warn(`[SW] 检测到新版本: ${localBuildId} → ${remoteBuildId}, 正在清理...`);

      const registrations = await navigator.serviceWorker.getRegistrations();
      for (const reg of registrations) {
        await reg.unregister()
      }

      const cacheNames = await caches.keys();
      await Promise.all(cacheNames.map(name => caches.delete(name)))

      localStorage.removeItem(SW_REGISTERED_KEY);

      await new Promise(r => setTimeout(r, 100));
      console.info('[SW] 旧版本已清理, 即将 reload 到新版本')
      location.reload()
      return;
    }

    const registration = await navigator.serviceWorker.register('/sw.js', {
      updateViaCache: 'none',
    });

    localStorage.setItem(SW_VERSION_KEY, remoteBuildId);
    localStorage.setItem(SW_REGISTERED_KEY, '1')

    registration.addEventListener('updatefound', () => {
      const newSW = registration.installing
      if (!newSW) return
      console.info('[SW] 新版本下载中...')
      newSW.addEventListener('statechange', () => {
        if (newSW.state === 'installed' && navigator.serviceWorker.controller) {
          console.info('[SW] 新版本已就绪, 正在 activate...');
          newSW.postMessage({ type: 'SKIP_WAITING' })
        }
      })
    });

    registration.update().catch(() => {});

    console.info(`[SW] 已注册, buildId=${remoteBuildId}`)
  } catch (err) {
    console.warn('[SW] 注册失败 (不影响功能):', err.message);
  }
}

if (document.readyState === 'complete') {
  manageServiceWorker()
} else {
  window.addEventListener('load', manageServiceWorker, { once: true })
}
