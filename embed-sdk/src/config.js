/**
 * 配置解析
 * 从 <script data-*> 属性解析 widget 配置
 */

const DEFAULTS = {
  appKey: '',           // 可选：渠道 AppKey（私域部署可不传，使用默认 channel）
  channelId: '',        // 可选：直接指定 channel_id（与 appKey 二选一）
  apiBaseURL: '',       // 默认同源
  position: 'bottom-right', // bottom-right | bottom-left
  color: '#1989fa',
  title: '在线客服',
  zIndex: 9999,
  offsetX: 24,
  offsetY: 24,
  width: 380,
  height: 560
}

export function parseConfig() {
  // 1. 找当前 script 标签
  const script = document.currentScript || (function () {
    const scripts = document.getElementsByTagName('script')
    for (let i = scripts.length - 1; i >= 0; i--) {
      if (scripts[i].src && scripts[i].src.includes('chat-widget')) {
        return scripts[i]
      }
    }
    return null
  })()

  const config = { ...DEFAULTS }

  if (script) {
    // 读取 data-* 属性
    if (script.dataset.appKey) config.appKey = script.dataset.appKey
    if (script.dataset.channelId) config.channelId = script.dataset.channelId
    if (script.dataset.apiBaseUrl) config.apiBaseURL = script.dataset.apiBaseUrl
    if (script.dataset.position) config.position = script.dataset.position
    if (script.dataset.color) config.color = script.dataset.color
    if (script.dataset.title) config.title = script.dataset.title
    if (script.dataset.zIndex) config.zIndex = parseInt(script.dataset.zIndex, 10) || DEFAULTS.zIndex
    if (script.dataset.offsetX) config.offsetX = parseInt(script.dataset.offsetX, 10) || DEFAULTS.offsetX
    if (script.dataset.offsetY) config.offsetY = parseInt(script.dataset.offsetY, 10) || DEFAULTS.offsetY
  }

  // 2. 兼容 window.MarketingChatWidgetConfig 覆盖
  if (typeof window !== 'undefined' && window.MarketingChatWidgetConfig) {
    Object.assign(config, window.MarketingChatWidgetConfig)
  }

  // 3. 兼容 query 参数覆盖（部分场景用）
  const params = new URLSearchParams(window.location.search)
  if (params.get('app_key')) config.appKey = params.get('app_key')
  if (params.get('channel_id')) config.channelId = params.get('channel_id')

  // 4. 解析 apiBaseURL
  if (!config.apiBaseURL) {
    // 默认使用 script 标签的同源（widget 部署在哪里就用哪里的 chat 后端）
    if (script && script.src) {
      try {
        const url = new URL(script.src)
        config.apiBaseURL = url.origin
      } catch {
        config.apiBaseURL = window.location.origin
      }
    } else {
      config.apiBaseURL = window.location.origin
    }
  }

  // 5. 私域部署：appKey / channelId 都不是必填
  //    缺失时后端会使用默认 channel "default"，访客照样可以发起对话
  return config
}
