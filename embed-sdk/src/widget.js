/**
 * @file marketing-chat-widget 浮标 SDK 入口
 * @description HiveMtk 用户端 Chat Widget 嵌入 SDK(ADR-011)
 *
 * 用法(私域部署,最简集成):
 *   <script src="https://your-host/marketing-chat-widget.iife.js"></script>
 *
 * 可选 data-* 属性(多渠道 / 个性化时使用):
 *   - data-app-key        可选(私域部署不需要,缺失时使用默认 channel)
 *   - data-channel-id     可选(直接指定 channel_id)
 *   - data-api-base-url   可选,默认同源
 *   - data-position       bottom-right | bottom-left,默认 bottom-right
 *   - data-color          浮标颜色(hex),默认 #1989fa
 *   - data-title          聊天窗标题,默认 "在线客服"
 *   - data-welcome        访客打开聊天窗时的欢迎语
 *   - data-lang           zh-CN | en-US
 *   - data-visitor-id-key localStorage 中访客 UUID 的 key
 *   - data-z-index        层级,默认 9999
 *
 * 设计原则(私域部署模式):
 *   1. AppKey / Channel 都不是必填;用户自己部署本系统后,浮标直接可用
 *   2. 浮标 + 聊天窗所有颜色 / 装饰保持极简白底
 *   3. WebSocket 无鉴权,自己网站直连
 *   4. 自动重连 + 离线消息补发(连接时拉取)
 *
 * 设计原则(跨域):
 *   1. 父端维护 allowedOrigins 白名单(自动 = [apiBaseURL, window.location.origin])
 *   2. iframe 端用具体 origin 发送 postMessage,不用 '*'
 *   3. 所有事件回调(events.onMessage 等)对外暴露,方便高级用户做埋点
 */

import { parseConfig } from './config.js'
import { FloatingButton } from './floating-button.js'
import { IframePanel } from './iframe-panel.js'

/**
 * @typedef {import('./config.js').McwConfig} McwConfig
 * @typedef {import('./config.js').McwEvents} McwEvents
 */

/**
 * iframe -> 父端 postMessage 消息类型
 * @typedef {Object} McwMessage
 * @property {string} type       消息类型: 'mcw-config' | 'mcw-unread' | 'mcw-ready' | 'mcw-message' | 'chat-widget-close'
 * @property {*}      [payload]  消息负载
 */

/**
 * 营销客服浮标 SDK 主类
 */
class MarketingChatWidget {
  /**
   * @param {McwConfig} config
   */
  constructor(config) {
    /** @type {McwConfig} */
    this.config = config
    /** @type {FloatingButton|null} */
    this.button = null
    /** @type {IframePanel|null} */
    this.panel = null
    /** @type {boolean} */
    this.opened = false
  }

  /**
   * 初始化:创建浮标 + iframe 面板,挂载到 DOM
   * @returns {void}
   */
  init() {
    // appKey / channelId 都不强制要求,缺失时后端使用默认 channel
    // 控制台不再报"缺少"错误,仅做 info 提示
    if (!this.config.appKey && !this.config.channelId) {
      console.info('[MarketingChatWidget] 未指定 appKey/channelId,使用默认 channel(私域部署模式)')
    }

    // 创建浮标
    this.button = new FloatingButton({
      color: this.config.color,
      position: this.config.position,
      zIndex: this.config.zIndex,
      offsetX: this.config.offsetX,
      offsetY: this.config.offsetY,
      onClick: (opened) => this.toggle(opened)
    })

    // 创建 iframe 面板
    this.panel = new IframePanel({
      apiBaseURL: this.config.apiBaseURL,
      appKey: this.config.appKey,
      channelId: this.config.channelId,
      position: this.config.position,
      color: this.config.color,
      title: this.config.title,
      welcome: this.config.welcome,
      lang: this.config.lang,
      visitorIdKey: this.config.visitorIdKey,
      zIndex: this.config.zIndex,
      offsetX: this.config.offsetX,
      offsetY: this.config.offsetY,
      width: this.config.width,
      height: this.config.height,
      allowedOrigins: this.config.allowedOrigins,
      onClose: () => {
        this.opened = false
        if (this.button) this.button.setOpen(false)
        this._fireEvent('onClose')
      }
    })

    this.button.mount()

    // 监听 iframe 上报的消息
    this._bindMessageListener()

    // 触发 onReady 事件
    this._fireEvent('onReady', {
      apiBaseURL: this.config.apiBaseURL,
      channelRef: this.panel.getChannelRef()
    })
  }

  /**
   * 切换聊天窗开关
   * @param {boolean} opened
   * @returns {void}
   */
  toggle(opened) {
    this.opened = opened
    if (opened) {
      this.panel.show()
      this._fireEvent('onOpen')
    } else {
      this.panel.hide()
      this._fireEvent('onClose')
    }
  }

  /**
   * 打开聊天窗
   * @returns {void}
   */
  open() {
    if (this.opened) return
    this.opened = true
    if (this.button) this.button.setOpen(true)
    if (this.panel) this.panel.show()
    this._fireEvent('onOpen')
  }

  /**
   * 关闭聊天窗
   * @returns {void}
   */
  close() {
    if (!this.opened) return
    this.opened = false
    if (this.button) this.button.setOpen(false)
    if (this.panel) this.panel.hide()
    this._fireEvent('onClose')
  }

  /**
   * 完全销毁(移除浮标 + iframe + 监听器)
   * @returns {void}
   */
  destroy() {
    if (this._onWindowMessage) {
      window.removeEventListener('message', this._onWindowMessage)
      this._onWindowMessage = null
    }
    if (this.button) this.button.unmount()
    if (this.panel) this.panel.destroy()
    this.button = null
    this.panel = null
  }

  /**
   * 触发用户在 config.events 中注册的回调
   * @param {keyof McwEvents} eventName
   * @param {*} [payload]
   * @returns {void}
   * @private
   */
  _fireEvent(eventName, payload) {
    const events = (this.config && this.config.events) || {}
    const fn = events[eventName]
    if (typeof fn === 'function') {
      try {
        fn(payload)
      } catch (err) {
        console.error('[MarketingChatWidget] event ' + eventName + ' error:', err)
      }
    }
  }

  /**
   * 监听 iframe 上报的所有 postMessage 消息
   * (未读数 / onMessage 透传)
   * @returns {void}
   * @private
   */
  _bindMessageListener() {
    this._onWindowMessage = (e) => {
      // 跨域安全:仅允许白名单 origin
      if (this.config.allowedOrigins && this.config.allowedOrigins.length > 0) {
        if (!this.config.allowedOrigins.includes(e.origin)) return
      }
      const data = e.data
      if (!data || typeof data !== 'object') return

      if (data.type === 'mcw-unread') {
        if (this.button) this.button.setUnread(data.count || 0)
        this._fireEvent('onUnread', { count: data.count || 0 })
        return
      }

      // 透传其他消息给用户回调
      this._fireEvent('onMessage', { type: data.type, payload: data.payload })
    }
    window.addEventListener('message', this._onWindowMessage)
  }
}

// 启动
const config = parseConfig()
const widget = new MarketingChatWidget(config)
widget.init()

// 暴露到全局,便于高级用户控制
if (typeof window !== 'undefined') {
  window.MarketingChatWidget = MarketingChatWidget
  window.mcwInstance = widget
}

export default MarketingChatWidget
export { MarketingChatWidget }
