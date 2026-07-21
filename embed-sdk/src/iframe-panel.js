/**
 * Iframe 聊天窗
 * 通过 iframe 加载嵌入页面，与浮标 SDK 通过 postMessage 通信
 *
 * 私域部署（2026-07-17 优化）：
 *   - appKey 缺失时跳转到 /chat/embed/default
 *   - 极简白底，无装饰
 */

export class IframePanel {
  constructor(options) {
    this.apiBaseURL = options.apiBaseURL
    this.appKey = options.appKey || ''
    this.channelId = options.channelId || ''
    this.position = options.position || 'bottom-right'
    this.color = options.color || '#1989fa'
    this.title = options.title || '在线客服'
    this.zIndex = options.zIndex || 9999
    this.offsetX = options.offsetX || 24
    this.offsetY = options.offsetY || 24
    this.width = options.width || 380
    this.height = options.height || 560
    this.onClose = options.onClose || (() => {})

    this.iframe = null
    this.shown = false
    this.messageHandler = null
  }

  // 计算最终跳转的 channel 标识：优先 channelId > appKey > default
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
    // 触发 iframe 内部页面渲染
    setTimeout(() => {
      try {
        this.iframe.contentWindow.postMessage({
          type: 'mcw-config',
          payload: {
            appKey: this.appKey,
            channelId: this.channelId,
            apiBaseURL: this.apiBaseURL,
            color: this.color,
            title: this.title
          }
        }, (this.apiBaseURL ? new URL(this.apiBaseURL).origin : window.location.origin))
      } catch (e) {
        // ignore
      }
      // 移动端视觉视口适配（键盘弹出时跟随，避免输入框被遮挡）
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
    // URL 改为：/chat/embed/{channelRef}#/chat/embed/{channelRef}
    // - 第一个 {channelRef} 是 server 端 SPA fallback 路径（必须和 server 路由一致）
    // - 第二个 {channelRef}（# 后面）是 Vue Router hash 模式需要的内部路径
    //   hash 模式 SPA 解析 # 后的内容为路由路径，server 只看 /chat/embed/{channelRef}
    const channelRef = this.getChannelRef()
    iframe.src = `${this.apiBaseURL}/chat/embed/${encodeURIComponent(channelRef)}#/chat/embed/${encodeURIComponent(channelRef)}`
    iframe.allow = 'clipboard-write'
    iframe.style.cssText = this.getStyle('none')
    document.body.appendChild(iframe)
    this.iframe = iframe

    // 监听 iframe 消息
    this.messageHandler = (event) => {
      // 验证 origin
      try {
        const allowedOrigin = new URL(this.apiBaseURL).origin
        if (event.origin !== allowedOrigin) return
      } catch {
        return
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
    const isLeft = this.position === 'bottom-left'
    // 浮标 + 间距
    const btnSize = 56
    const gap = 16
    const sideOffset = this.offsetX + btnSize + gap
    const bottomOffset = this.offsetY

    // 移动端：全屏
    const isMobile = window.innerWidth <= 480
    if (isMobile) {
      return [
        'position: fixed',
        'top: 0',
        'left: 0',
        'right: 0',
        'bottom: 0',
        `width: 100vw`,
        `height: 100vh`,
        `z-index: ${this.zIndex + 1}`,
        'border: none',
        `display: ${display}`,
        'background: #fff'
      ].join(';')
    }

    return [
      'position: fixed',
      `bottom: ${bottomOffset}px`,
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

  // 移动端视觉视口适配：键盘弹出时 visualViewport 高度会缩小，
  // 监听 resize/scroll 让 iframe 跟随视觉视口，避免输入框被键盘遮挡
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
