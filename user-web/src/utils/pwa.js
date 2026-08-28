/**
 * PWA 评估与实施（USR-PF-05）
 * 依赖：vite-plugin-pwa 0.21+（已在 package.json）
 *
 * 配置 vite.config.js + manifest + Service Worker
 */

// vite.config.js 中 PWA 插件配置（参考）
const PWA_VITE_CONFIG = `
// vite.config.js 添加：
import { VitePWA } from 'vite-plugin-pwa'

plugins: [
  // ...其他插件
  VitePWA({
    registerType: 'autoUpdate',
    workbox: {
      globPatterns: ['**/*.{js,css,html,ico,png,svg,woff2}'],
      runtimeCaching: [
        {
          // API GET 请求 - NetworkFirst（USR-PF-05）
          urlPattern: /^\\/api\\/(?!ws|bridge\\/outbox).*$/,
          handler: 'NetworkFirst',
          options: {
            cacheName: 'api-cache',
            networkTimeoutSeconds: 5,
            expiration: { maxEntries: 100, maxAgeSeconds: 5 * 60 }
          }
        },
        {
          // 静态资源 - CacheFirst
          urlPattern: /\\.(?:png|jpg|jpeg|svg|webp|woff2)$/,
          handler: 'CacheFirst',
          options: {
            cacheName: 'image-cache',
            expiration: { maxEntries: 200, maxAgeSeconds: 30 * 24 * 60 * 60 }
          }
        },
        {
          // 字体 - CacheFirst
          urlPattern: /\\.(?:woff|woff2|ttf|eot)$/,
          handler: 'CacheFirst',
          options: { cacheName: 'font-cache' }
        }
      ]
    },
    manifest: {
      name: 'HiveMtk',
      short_name: 'HiveMtk',
      description: '营销 + 客服 + AI 智能体工作台',
      theme_color: '#4F46E5',
      background_color: '#F8FAFC',
      display: 'standalone',
      icons: [
        { src: 'pwa-192x192.png', sizes: '192x192', type: 'image/png' },
        { src: 'pwa-512x512.png', sizes: '512x512', type: 'image/png' }
      ]
    }
  })
]
`

// 桌面快捷方式配置
const SHORTCUTS = [
  // R1-D1 修复: 统一收件箱入口已废弃移除
  { name: 'AI 智能体', url: '/aiAgent/list', icon: '/icons/ai.png' },
  { name: '客户 360', url: '/customer360/list', icon: '/icons/customer.png' },
  { name: '数据大屏', url: '/dashboardScreen/list', icon: '/icons/dashboard.png' }
]

// 推送通知
export const requestPushPermission = async () => {
  if (!('Notification' in window)) return 'unsupported'
  if (Notification.permission === 'granted') return 'granted'
  if (Notification.permission === 'denied') return 'denied'
  const result = await Notification.requestPermission()
  return result
}

export const showNotification = (title, options = {}) => {
  if (Notification.permission === 'granted') {
    return new Notification(title, { icon: '/icons/icon-192.png', ...options })
  }
  return null
}

// 离线检测
export const useOnlineStatus = () => {
  const isOnline = ref(typeof navigator !== 'undefined' ? navigator.onLine : true)
  if (typeof window !== 'undefined') {
    window.addEventListener('online', () => (isOnline.value = true))
    window.addEventListener('offline', () => (isOnline.value = false))
  }
  return isOnline
}

import { ref }
export default { PWA_VITE_CONFIG, SHORTCUTS, requestPushPermission, showNotification, useOnlineStatus }
