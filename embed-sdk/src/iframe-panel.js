


/**
 * 浮标按钮类(管理 iframe 聊天窗的创建 / 显示 / 隐藏 / 销毁)
 */
export class IframePanel {
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
    // USR-EM-01: Embedded 模式（容器内渲染，参考 Zendesk Web Widget）
    this.mode = options.mode || 'floating' // 'floating' | 'embedded'
    this.targetElement = options.targetElement || null
    this.cspNonce = options.cspNonce || ''

    this.iframe = null
    this.shown = false
    this.messageHandler = null
  }

  getChannelRef() {
    if (this.channelId) return this.channelId
    if (this.appKey) return this.appKey
    return 'default'
  }

  show() {
    if (this.shown) return
    if (!this.iframe) {
      this.create()
    }
    this.iframe.style.display = 'block'
    this.shown = true
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
      }
      this.setupVisualViewport()
    }, 100)
  }

  hide() {
    if (!this.shown) return
    if (this.iframe) {
      this.iframe.style.display = 'none'
    }
    this.shown = false
  }

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

  create() {
    const iframe = document.createElement('iframe')
    iframe.className = 'mcw-iframe'
    iframe.title = this.title
    const channelRef = this.getChannelRef()
    iframe.src = `${this.apiBaseURL}/chat/embed/${encodeURIComponent(channelRef)}#/chat/embed/${encodeURIComponent(channelRef)}`
    iframe.allow = 'clipboard-write'
    // USR-EM-03: CSP nonce
    if (this.cspNonce) {
      iframe.setAttribute('nonce', this.cspNonce)
    }
    iframe.style.cssText = this.getStyle('none')

    // USR-EM-01: Embedded 模式挂到目标容器
    if (this.mode === 'embedded' && this.targetElement) {
      const target = typeof this.targetElement === 'string'
        ? document.querySelector(this.targetElement)
        : this.targetElement
      if (target) {
        target.appendChild(iframe)
      } else {
        console.warn('[MarketingChatWidget] targetElement 不存在，回退到 body')
        document.body.appendChild(iframe)
      }
    } else {
      document.body.appendChild(iframe)
    }
    this.iframe = iframe

    this.messageHandler = (event) => {
      if (this.allowedOrigins && this.allowedOrigins.length > 0) {
        if (!this.allowedOrigins.includes(event.origin)) {
          return
        }
      } else {
        try {
          const allowedOrigin = new URL(this.apiBaseURL).origin
          if (event.origin !== allowedOrigin) return
        } catch (_) {
          return
        }
      }

      const data = event.data
      if (!data || typeof data !== 'object') return

      if (data.type === 'chat-widget-close') {
        this.hide()
        this.onClose()
      }
    }
    window.addEventListener('message', this.messageHandler)
  }

  getStyle(display) {
    // USR-EM-01: Embedded 模式用 100% 容器尺寸
    if (this.mode === 'embedded') {
      return [
        'border: none',
        'width: 100%',
        'height: 100%',
        'min-height: 400px',
        `z-index: ${this.zIndex}`,
        `display: ${display}`,
        'background: #fff',
        'border-radius: 8px'
      ].join(';')
    }

    const isLeft = this.position === 'bottom-left'
    const isMobile = (typeof window !== 'undefined') && window.innerWidth <= 480

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

    // USR-EM-02: 用逻辑属性（inset-inline-*）自动 RTL 镜像
    return [
      'position: fixed',
      `bottom: ${this.offsetY}px`,
      isLeft ? `inset-inline-start: ${this.offsetX}px` : `inset-inline-end: ${this.offsetX}px`,
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

  clearVisualViewport() {
    if (this._vv && this._vvApply) {
      this._vv.removeEventListener('resize', this._vvApply)
      this._vv.removeEventListener('scroll', this._vvApply)
      this._vv = null
      this._vvApply = null
    }
  }
}

