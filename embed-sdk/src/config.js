


const DEFAULTS = {
  appKey: '',
  channelId: '',
  apiBaseURL: '',
  position: 'bottom-right',
  color: '#1989fa',
  title: '在线客服',
  welcome: '您好,请问有什么可以帮您?',
  lang: 'zh-CN',
  visitorIdKey: 'mtk_visitor_id',
  zIndex: 9999,
  offsetX: 24,
  offsetY: 24,
  width: 380,
  height: 560,
  allowedOrigins: null,   
  events: {}              
};

function readDataAttrs(script) {
  if (!script || !script.dataset) return {}
  const d = script.dataset
  const out = {}
  if (d.appKey)        out.appKey = d.appKey
  if (d.channelId)     out.channelId = d.channelId
  if (d.apiBaseUrl)    out.apiBaseURL = d.apiBaseUrl
  if (d.position)      out.position = d.position
  if (d.color)         out.color = d.color
  if (d.title)         out.title = d.title
  if (d.welcome)       out.welcome = d.welcome
  if (d.lang)          out.lang = d.lang
  if (d.visitorIdKey)  out.visitorIdKey = d.visitorIdKey
  if (d.zIndex)        out.zIndex = parseInt(d.zIndex, 10) || DEFAULTS.zIndex
  if (d.offsetX)       out.offsetX = parseInt(d.offsetX, 10) || DEFAULTS.offsetX
  if (d.offsetY)       out.offsetY = parseInt(d.offsetY, 10) || DEFAULTS.offsetY
  if (d.width)         out.width = parseInt(d.width, 10) || DEFAULTS.width
  if (d.height)        out.height = parseInt(d.height, 10) || DEFAULTS.height
  return out
}

function readQueryParams() {
  if (typeof window === 'undefined') return {}
  const params = new URLSearchParams(window.location.search)
  const out = {}
  if (params.get('app_key'))     out.appKey = params.get('app_key')
  if (params.get('channel_id'))  out.channelId = params.get('channel_id')
  if (params.get('lang'))        out.lang = params.get('lang')
  return out
}

function resolveApiBaseURL(script) {
  if (script && script.src) {
    try {
      return new URL(script.src).origin
    } catch (_) {  }
  }
  if (typeof window !== 'undefined') {
    return window.location.origin
  }
  return ''
}

export function parseConfig() {
  const script = (typeof document !== 'undefined')
    ? (document.currentScript || (function () {
      const scripts = document.getElementsByTagName('script')
      for (let i = scripts.length - 1; i >= 0; i--) {
        if (scripts[i].src && scripts[i].src.includes('chat-widget')) {
          return scripts[i]
        }
      }
      return null
    })())
    : null

  const config = { ...DEFAULTS }
  Object.assign(config, readDataAttrs(script))
  Object.assign(config, readQueryParams())

  if (typeof window !== 'undefined' && window.MarketingChatWidgetConfig) {
    Object.assign(config, window.MarketingChatWidgetConfig)
  }

  if (!config.apiBaseURL) {
    config.apiBaseURL = resolveApiBaseURL(script)
  }

  if (!config.allowedOrigins || !Array.isArray(config.allowedOrigins) || config.allowedOrigins.length === 0) {
    const origins = []
    try { if (config.apiBaseURL) origins.push(new URL(config.apiBaseURL).origin) } catch (_) {}
    try { if (typeof window !== 'undefined') origins.push(window.location.origin) } catch (_) {}
    config.allowedOrigins = Array.from(new Set(origins))
  }

  return config
}

export { DEFAULTS }

