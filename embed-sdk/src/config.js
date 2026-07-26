/**
 * @file 配置解析
 * @description 从 <script data-*> 属性 / window.MarketingChatWidgetConfig / query 参数解析 widget 配置
 *
 * @example 基础用法
 * <script
 *   src="https://chat.example.com/embed/marketing-chat-widget.iife.js"
 *   data-app-key="ak_xxx"
 *   data-color="#1989fa"
 *   data-welcome="您好,请问有什么可以帮您?"
 * ></script>
 *
 * @example 全局变量配置
 * <script>
 *   window.MarketingChatWidgetConfig = {
 *     appKey: 'ak_xxx',
 *     apiBaseURL: 'https://api.example.com',
 *     color: '#ff6b35',
 *     welcome: 'Hi, how can I help you?',
 *     lang: 'en-US',
 *     events: {
 *       onMessage: function(payload) { console.log('received:', payload) }
 *     }
 *   }
 * </script>
 * <script src="https://chat.example.com/embed/marketing-chat-widget.iife.js"></script>
 *
 * 解析优先级:window.MarketingChatWidgetConfig > query 参数 > data-* 属性 > 内置默认值
 */

/**
 * 事件回调集合
 * @typedef {Object} McwEvents
 * @property {Function} [onOpen]    聊天窗打开时触发
 * @property {Function} [onClose]   聊天窗关闭时触发
 * @property {Function} [onUnread]  未读消息数变化时触发,参数:{count:number}
 * @property {Function} [onMessage] 收到新消息时触发,参数:{type:string, payload:object}
 * @property {Function} [onReady]   SDK 初始化完成时触发,参数:{apiBaseURL:string, channelRef:string}
 */

/**
 * Widget 完整配置项
 * @typedef {Object} McwConfig
 * @property {string}    [appKey='']            渠道 AppKey(私域部署可省略,自动用 `default` 渠道)
 * @property {string}    [channelId='']         直接指定 channel_id(与 appKey 二选一)
 * @property {string}    [apiBaseURL='']        API 基础 URL;默认使用 script 同源或 window.location.origin
 * @property {string}    [position='bottom-right']  浮标位置:bottom-right | bottom-left
 * @property {string}    [color='#1989fa']      浮标主色(hex)
 * @property {string}    [title='在线客服']     聊天窗标题
 * @property {string}    [welcome='您好,请问有什么可以帮您?']  访客打开聊天窗时的欢迎语
 * @property {string}    [lang='zh-CN']         语言:zh-CN | en-US
 * @property {string}    [visitorIdKey='mtk_visitor_id']  localStorage 中访客 UUID 的 key
 * @property {number}    [zIndex=9999]          浮标 / iframe 层级
 * @property {number}    [offsetX=24]           浮标水平边距(px)
 * @property {number}    [offsetY=24]           浮标垂直边距(px)
 * @property {number}    [width=380]            聊天窗宽度(px),移动端自动全屏
 * @property {number}    [height=560]           聊天窗高度(px),移动端自动全屏
 * @property {string[]}  [allowedOrigins]       允许的 postMessage origin 列表;留空则自动 = [apiBaseURL, window.location.origin]
 * @property {McwEvents} [events]               事件回调
 */

/** @type {McwConfig} */
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
  allowedOrigins: null,   // null 表示自动从 apiBaseURL + window.location.origin 推导
  events: {}              // 用户回调
}

/**
 * 解析 <script data-*> 属性
 * @param {HTMLScriptElement|null} script
 * @returns {Partial<McwConfig>}
 */
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

/**
 * 解析 query string(部分场景使用)
 * @returns {Partial<McwConfig>}
 */
function readQueryParams() {
  if (typeof window === 'undefined') return {}
  const params = new URLSearchParams(window.location.search)
  const out = {}
  if (params.get('app_key'))     out.appKey = params.get('app_key')
  if (params.get('channel_id'))  out.channelId = params.get('channel_id')
  if (params.get('lang'))        out.lang = params.get('lang')
  return out
}

/**
 * 解析 apiBaseURL(默认使用 script 同源 / window.location.origin)
 * @param {HTMLScriptElement|null} script
 * @returns {string}
 */
function resolveApiBaseURL(script) {
  if (script && script.src) {
    try {
      return new URL(script.src).origin
    } catch (_) { /* fall through */ }
  }
  if (typeof window !== 'undefined') {
    return window.location.origin
  }
  return ''
}

/**
 * 解析最终配置
 * @returns {McwConfig}
 */
export function parseConfig() {
  // 1. 找当前 script 标签
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

  // 2. 按优先级合并
  const config = { ...DEFAULTS }
  Object.assign(config, readDataAttrs(script))
  Object.assign(config, readQueryParams())

  if (typeof window !== 'undefined' && window.MarketingChatWidgetConfig) {
    Object.assign(config, window.MarketingChatWidgetConfig)
  }

  // 3. 兜底 apiBaseURL
  if (!config.apiBaseURL) {
    config.apiBaseURL = resolveApiBaseURL(script)
  }

  // 4. 自动推导 allowedOrigins
  if (!config.allowedOrigins || !Array.isArray(config.allowedOrigins) || config.allowedOrigins.length === 0) {
    const origins = []
    try { if (config.apiBaseURL) origins.push(new URL(config.apiBaseURL).origin) } catch (_) {}
    try { if (typeof window !== 'undefined') origins.push(window.location.origin) } catch (_) {}
    config.allowedOrigins = Array.from(new Set(origins))
  }

  return config
}

export { DEFAULTS }
