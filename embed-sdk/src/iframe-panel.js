/**
 * @file Iframe 聊天窗
 * @description 通过 iframe 加载嵌入页面,与浮标 SDK 通过 postMessage 通信
 *
 * 私域部署(2026-07-17 优化):
 *   - appKey 缺失时跳转到 /chat/embed/default
 *   - 极简白底,无装饰
 *
 * 跨域(2026-07-21 修复):
 *   - 父端用 allowedOrigins 列表(自动包含 apiBaseURL + window.location.origin)
 *     替代严格的 `=== apiBaseURL.origin`,避免跨域部署下父子通信被拒
 *   - iframe 端用具体 origin 发送,不用 '*'
 *   - chat-widget-close 关闭消息走同一白名单校验,与 onMessage 等保持一致
 */

/**
 * @typedef {import('./config.js').McwConfig} McwConfig
 */

/**
 * Iframe 面板构造选项
 * @typedef {Object} IframePanelOptions
 * @property {string}   apiBaseURL
 * @property {string}   [appKey]
 * @property {string}   [channelId]
 * @property {string}   [position]
 * @property {string}   [color]
 * @property {string}   [title]
 * @property {string}   [welcome]
 * @property {string}   [lang]
 * @property {string}   [visitorIdKey]
 * @property {number}   [zIndex]
 * @property {number}   [offsetX]
 * @property {number}   [offsetY]
 * @property {number}   [width]
 * @property {number}   [height]
 * @property {string[]} [allowedOrigins]
 * @property {Function} [onClose]
 */

/**
 * 浮标按钮类(管理 iframe 聊天窗的创建 / 显示 / 隐藏 / 销毁)
 */
export class IframePanel {
  /**
   * @param {IframePanelOptions} options
   */
  constructor(options) {
    this.apiBaseURL = options.apiBaseURL
    this.appKey = options.appKey || ''
    this.channelId = options.channelId || ''
    this.position = options.position || 'bottom-right'
    this.color = options.color || '#1989fa'
    this.title = options.title || '在线客服'
    this.welcome = options.welcome || ''
    this.lang = options.lang || 'zh-CN'
    this.visitorIdKey = options.visitorIdKey || 'mtk_visitor_id'
    this.zIndex = options.zIndex || 9999
    this.offsetX = options.offsetX || 24
    this.offsetY = options.offsetY || 24
    this.width = options.width || 380
    this.height = options.height || 560
    this.allowedOrigins = options.allowedOrigins || []
    this.onClose = options.onClose || (() => {})

    /** @type {HTMLIFrameElement|null} */
    this.iframe = null
    this.shown = false
    /** @type {Function|null} */
    this.messageHandler = null
  }

  /**
   * 计算最终跳转的 channel 标识
   * @returns {string}
   */
  getChannelRef() {
    if (this.channelId) return this.channelId
    if (this.appKey) return this.appKey
    return 'default'
  }

  /**
   * 显示聊天窗
   * @returns {void}
   */
  show() {
    if (this.shown) return
    if (!this.iframe) {
      this.create()
    }
    this.iframe.style.display = 'block'
    this.shown = true
    // 触发 iframe 内部页面渲染
    setTimeout(() => {
      try {
        const targetOrigin = (() => {
          try { return new URL(this.apiBaseURL).origin } catch (_) { return window.location.origin }
        })()
        this.iframe.contentWindow.postMessage({
          type: 'mcw-config',
          payload: {
            appKey: this.appKey,
            channelId: this.channelId,
            channelRef: this.getChannelRef(),
            apiBaseURL: this.apiBaseURL,
            color: this.color,
            title: this.title,
            welcome: this.welcome,
            lang: this.lang,
            visitorIdKey: this.visitorIdKey
          }
        }, targetOrigin)
      } catch (e) {
        // ignore
      }
      // 移动端视觉视口适配
      this.setupVisualViewport()
    }, 100)
  }

  /**
   * 隐藏聊天窗
   * @returns {void}
   */
  hide() {
    if (!this.shown) return
    if (this.iframe) {
      this.iframe.style.display = 'none'
    }
    this.shown = false
  }

  /**
   * 销毁(移除 iframe + 消息监听)
   * @returns {void}
   */
  destroy() {
    this.clearVisualViewport()
    if (this.iframe) {
      this.iframe.remove()
      this.iframe = null
    }
    if (this.messageHandler) {
      window.removeEventListener('message', this.messageHandler)
      this.messageHandler = null
    }
  }

  /**
   * 创建 iframe DOM + 注册消息监听
   * @private
   */
  create() {
    const iframe = document.createElement('iframe')
    iframe.className = 'mcw-iframe'
    iframe.title = this.title
    const channelRef = this.getChannelRef()
    iframe.src = `${this.apiBaseURL}/chat/embed/${encodeURIComponent(channelRef)}#/chat/embed/${encodeURIComponent(channelRef)}`
    iframe.allow = 'clipboard-write'
    iframe.style.cssText = this.getStyle('none')
    document.body.appendChild(iframe)
    this.iframe = iframe

    // 监听 iframe 消息(2026-07-21:统一使用 allowedOrigins 白名单)
    // 关闭消息 / 业务消息 / 任何 postMessage 都走同一校验,避免不一致漏洞
    this.messageHandler = (event) => {
      // 跨域安全:allowedOrigins 是父端预先配置的白名单
      if (this.allowedOrigins && this.allowedOrigins.length > 0) {
        if (!this.allowedOrigins.includes(event.origin)) {
          return
        }
      } else {
        // 兜底:仅允许 apiBaseURL origin
        try {
          const allowedOrigin = new URL(this.apiBaseURL).origin
          if (event.origin !== allowedOrigin) return
        } catch (_) {
          return
        }
      }

      const data = event.data
      if (!data || typeof data !== 'object') return

      // 关闭消息(由 iframe 内部聊天窗主动 postMessage)
      if (data.type === 'chat-widget-close') {
        this.hide()
        this.onClose()
      }
    }
    window.addEventListener('message', this.messageHandler)
  }

  /**
   * 计算 iframe 内联样式
   * @param {string} display  CSS display 值
   * @returns {string}
   * @private
   */
  getStyle(display) {
    const isLeft = this.position === 'bottom-left'
    const isMobile = (typeof window !== 'undefined') && window.innerWidth <= 480

    // 移动端:全屏
    if (isMobile) {
      return [
        'position: fixed',
        'top: 0',
        'left: 0',
        'right: 0',
        'bottom: 0',
        'width: 100vw',
        'height: 100vh',
        `z-index: ${this.zIndex + 1}`,
        'border: none',
        `display: ${display}`,
        'background: #fff'
      ].join(';')
    }

    return [
      'position: fixed',
      `bottom: ${this.offsetY}px`,
      isLeft ? `left: ${this.offsetX}px` : `right: ${this.offsetX}px`,
      `width: ${this.width}px`,
      `height: ${this.height}px`,
      `z-index: ${this.zIndex + 1}`,
      'border: none',
      'border-radius: 10px',
      'box-shadow: 0 4px 24px rgba(0, 0, 0, 0.15)',
      `display: ${display}`,
      'background: #fff'
    ].join(';')
  }

  /**
   * 移动端视觉视口适配:键盘弹出时跟随
   * @private
   */
  setupVisualViewport() {
    const vv = window.visualViewport
    if (!vv) return
    if (window.innerWidth > 480) return
    const apply = () => {
      if (!this.iframe) return
      this.iframe.style.position = 'fixed'
      this.iframe.style.top = vv.offsetTop + 'px'
      this.iframe.style.left = vv.offsetLeft + 'px'
      this.iframe.style.width = vv.width + 'px'
      this.iframe.style.height = vv.height + 'px'
      this.iframe.style.right = 'auto'
      this.iframe.style.bottom = 'auto'
    }
    apply()
    vv.addEventListener('resize', apply)
    vv.addEventListener('scroll', apply)
    this._vv = vv
    this._vvApply = apply
  }

  /**
   * 清理视觉视口监听
   * @private
   */
  clearVisualViewport() {
    if (this._vv && this._vvApply) {
      this._vv.removeEventListener('resize', this._vvApply)
      this._vv.removeEventListener('scroll', this._vvApply)
      this._vv = null
      this._vvApply = null
    }
  }
}
